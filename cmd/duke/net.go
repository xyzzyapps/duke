package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"shooter/internal/actions"
	"shooter/internal/agent"
	"shooter/internal/bot"
	"shooter/internal/doc"
	"shooter/internal/grid"
	"shooter/internal/netplay"
	"shooter/internal/render"
)

// peerState is what we know about one remote participant.
type peerState struct {
	name   string
	line   int
	col    int
	state  agent.State
	facing int
	chat   string
	chatT  float64 // seconds since the bubble appeared
	animT  float64 // animation clock for the remote sprite
}

// botState schedules the test bot's decisions.
type botState struct {
	r    *rand.Rand
	next float64 // seconds until the next action
}

// netCtl owns the LAN session: message pump, peer table, presence
// throttling, chat bubbles and the optional test bot. A nil *netCtl is a
// valid solo-play controller (every method is nil-safe), so the game loop
// carries no mode switches.
type netCtl struct {
	sess         netplay.Session // nil in solo play
	host         *netplay.Host   // non-nil when hosting (replies/drops)
	observed     *netplay.ObservedDoc
	selfID       string
	name         string
	peers        map[string]*peerState // keyed by participant id
	connPeer     map[int]string        // host: connection id -> participant id
	seq          int                   // host: relayed op sequence
	presT        float64               // presence throttle
	last         netplay.Peer          // last presence sent (change detection)
	bot          *botState
	localChat    *render.Bubble
	disconnected bool
	g            grid.Grid // cell -> world pixel mapping for peers
}

// newNetCtl returns a controller for the given session (nil-safe in solo).
func newNetCtl(sess netplay.Session, host *netplay.Host, observed *netplay.ObservedDoc,
	g grid.Grid, name string, withBot bool) *netCtl {
	if sess == nil {
		return nil
	}
	nc := &netCtl{
		sess:     sess,
		host:     host,
		observed: observed,
		selfID:   sess.SelfID(),
		name:     name,
		peers:    map[string]*peerState{},
		connPeer: map[int]string{},
		g:        g,
	}
	if withBot {
		nc.bot = &botState{r: rand.New(rand.NewSource(time.Now().UnixNano())), next: 1}
		log.Printf("bot online as %q", name)
	}
	return nc
}

// emit ships a local buffer mutation to the host (wrapper callback). The
// host broadcasts it; a client just forwards it.
func (nc *netCtl) emit(op netplay.DocOp) {
	if nc == nil || nc.sess == nil {
		return
	}
	nc.sess.Send(netplay.Msg{Kind: netplay.KindOp, Op: &op, Origin: nc.selfID})
}

// SendChat implements ui.Messenger: broadcast + local bubble.
func (nc *netCtl) SendChat(text string) {
	if nc == nil || nc.sess == nil {
		return
	}
	nc.sess.Send(netplay.Msg{
		Kind: netplay.KindChat,
		Peer: netplay.Peer{ID: nc.selfID, Name: nc.name, Chat: text},
	})
	nc.localChat = &render.Bubble{Text: text}
	log.Printf("chat: %s", text)
}

// Update pumps the session: applies incoming messages, throttles outgoing
// presence, ages bubbles and runs the bot. Call once per frame.
func (nc *netCtl) Update(dt float64, d doc.Document, engine engineAPI, v actions.View) {
	if nc == nil || nc.sess == nil {
		return
	}
	nc.pump(d)
	nc.presence(dt, v)
	nc.ageBubbles(dt)
	if nc.bot != nil {
		nc.botTick(dt, d, engine, v)
	}
}

// engineAPI is the small slice of the engine the net controller needs
// (actions.Engine satisfies it structurally).
type engineAPI interface {
	WalkTo(p doc.Pos)
	TypeRune(r rune)
	ShootAt(p doc.Pos)
	ShootWord(back bool)
	KillLine(back bool)
	Caret() doc.Pos
}

// pump drains every pending inbound message.
func (nc *netCtl) pump(d doc.Document) {
	for {
		select {
		case in := <-nc.sess.Incoming():
			nc.handle(d, in)
		default:
			return
		}
	}
}

// handle processes one inbound message (host and client share the shape;
// roles differ in relaying).
func (nc *netCtl) handle(d doc.Document, in netplay.Inbound) {
	m := in.Msg
	switch m.Kind {
	case netplay.KindHello:
		// Host only: register the peer and reply with a full snapshot.
		nc.peers[m.ID] = &peerState{name: m.Name}
		nc.connPeer[in.From] = m.ID
		if nc.host != nil {
			nc.host.SendTo(in.From, netplay.Msg{
				Kind:  netplay.KindWelcome,
				ID:    nc.selfID,
				Text:  d.Text(),
				Peers: nc.peerList(),
			})
			log.Printf("peer joined: %s", m.Name)
		}
	case netplay.KindWelcome:
		// Client only: the host's buffer wins.
		nc.observed.ApplyRemote(netplay.DocOp{Kind: "load", Text: m.Text})
		for _, p := range m.Peers {
			if p.ID != nc.selfID {
				nc.peers[p.ID] = peerFrom(p)
			}
		}
		log.Printf("joined: buffer is %d bytes, %d peers", len(m.Text), len(m.Peers))
	case netplay.KindOp:
		if m.Origin == nc.selfID {
			return // echo of our own edit
		}
		if m.Op == nil {
			return
		}
		nc.observed.ApplyRemote(*m.Op)
		if nc.host != nil {
			// Host relays to everyone else with a sequence stamp.
			nc.seq++
			m.Seq = nc.seq
			nc.host.BroadcastExcept(m, in.From)
		}
	case netplay.KindPresence:
		if m.Peer.ID == nc.selfID {
			return
		}
		p, ok := nc.peers[m.Peer.ID]
		if !ok {
			p = &peerState{}
			nc.peers[m.Peer.ID] = p
		}
		p.name, p.line, p.col = m.Peer.Name, m.Peer.Line, m.Peer.Col
		p.state, p.facing = parseState(m.Peer.State), m.Peer.Facing
		if nc.host != nil {
			nc.host.BroadcastExcept(m, in.From)
		}
	case netplay.KindChat:
		if m.Peer.ID == nc.selfID {
			return
		}
		p, ok := nc.peers[m.Peer.ID]
		if !ok {
			p = &peerState{name: m.Peer.Name}
			nc.peers[m.Peer.ID] = p
		}
		p.chat, p.chatT = m.Peer.Chat, 0
		log.Printf("%s says: %s", p.name, m.Peer.Chat)
		if nc.host != nil {
			nc.host.BroadcastExcept(m, in.From)
		}
	case netplay.KindLeave:
		id := m.ID
		if id == "" && nc.host != nil {
			id = nc.connPeer[in.From]
			nc.host.Drop(in.From)
		}
		if p, ok := nc.peers[id]; ok {
			log.Printf("peer left: %s", p.name)
			delete(nc.peers, id)
		}
		if nc.host != nil {
			nc.host.Broadcast(netplay.Msg{Kind: netplay.KindLeave, ID: id})
		}
	}
}

// peerList is the welcome roster (host + everyone already present).
func (nc *netCtl) peerList() []netplay.Peer {
	out := []netplay.Peer{{ID: nc.selfID, Name: nc.name}}
	for id, p := range nc.peers {
		out = append(out, netplay.Peer{
			ID: id, Name: p.name, Line: p.line, Col: p.col,
			State: p.state.String(), Facing: p.facing, Chat: p.chat,
		})
	}
	return out
}

// presence broadcasts our caret/pose a few times per second when it moved.
func (nc *netCtl) presence(dt float64, v actions.View) {
	nc.presT += dt
	if nc.presT < 0.25 {
		return
	}
	nc.presT = 0
	p := netplay.Peer{
		ID: nc.selfID, Name: nc.name,
		Line: v.Caret.Line, Col: v.Caret.Col,
		State: v.Agent.State.String(), Facing: v.Agent.Facing,
	}
	if p.Line == nc.last.Line && p.Col == nc.last.Col &&
		p.State == nc.last.State && p.Facing == nc.last.Facing {
		return
	}
	nc.last = p
	nc.sess.Send(netplay.Msg{Kind: netplay.KindPresence, Peer: p})
}

// ageBubbles advances chat/animation clocks and drops expired bubbles.
func (nc *netCtl) ageBubbles(dt float64) {
	if nc.localChat != nil {
		nc.localChat.T += dt
		if nc.localChat.T > render.BubbleLife {
			nc.localChat = nil
		}
	}
	for _, p := range nc.peers {
		p.animT += dt
		if p.chat != "" {
			p.chatT += dt
			if p.chatT > render.BubbleLife {
				p.chat = ""
			}
		}
	}
}

// botTick lets the test bot act on its schedule.
func (nc *netCtl) botTick(dt float64, d doc.Document, engine engineAPI, v actions.View) {
	nc.bot.next -= dt
	if nc.bot.next > 0 {
		return
	}
	nc.bot.next = 0.6 + nc.bot.r.Float64()*1.6
	act := bot.Think(d, v.Caret, nc.bot.r)
	switch act.Kind {
	case bot.ActWalk:
		engine.WalkTo(act.Pos)
	case bot.ActType:
		for _, ch := range act.Text {
			engine.TypeRune(ch)
		}
	case bot.ActShoot:
		engine.ShootAt(act.Pos)
	case bot.ActWord:
		engine.ShootWord(act.Back)
	case bot.ActLine:
		engine.KillLine(act.Back)
	case bot.ActChat:
		nc.SendChat(act.Chat)
	}
}

// actorWorld maps a cell to the sprite's world position.
func (nc *netCtl) actorWorld(cell doc.Pos) (float64, float64) {
	return nc.g.AgentOrigin(cell, agent.SpriteH)
}

// actors converts the peer table to drawable actors for the HUD.
func (nc *netCtl) actors() []render.Actor {
	if nc == nil {
		return nil
	}
	var out []render.Actor
	for _, p := range nc.peers {
		x, y := nc.actorWorld(doc.Pos{Line: p.line, Col: p.col})
		out = append(out, render.Actor{
			X: x, Y: y, Facing: p.facing, State: p.state, T: p.animT,
			Name: p.name, Chat: p.chat, ChatT: p.chatT,
		})
	}
	return out
}

// Closed reports a lost host (clients only) exactly once.
func (nc *netCtl) disconnectedOnce() bool {
	if nc == nil || nc.disconnected {
		return false
	}
	if c, ok := nc.sess.(*netplay.Client); ok && c.Closed() {
		nc.disconnected = true
		log.Printf("disconnected from host")
		return true
	}
	return false
}

// Close says goodbye and tears the session down (exit path).
func (nc *netCtl) Close() {
	if nc == nil || nc.sess == nil {
		return
	}
	nc.sess.Send(netplay.Msg{Kind: netplay.KindLeave, ID: nc.selfID})
	time.Sleep(120 * time.Millisecond) // let the writer flush
	_ = nc.sess.Close()
	log.Printf("session closed")
}

// parseState maps a wire state name back to an agent state.
func parseState(s string) agent.State {
	switch s {
	case "walk":
		return agent.StateWalk
	case "aim":
		return agent.StateAim
	case "fire":
		return agent.StateFire
	case "recoil":
		return agent.StateRecoil
	case "slash":
		return agent.StateSlash
	case "drag":
		return agent.StateDrag
	case "win":
		return agent.StateWin
	}
	return agent.StateIdle
}

// peerFrom converts a wire peer into local state.
func peerFrom(p netplay.Peer) *peerState {
	return &peerState{
		name: p.Name, line: p.Line, col: p.Col,
		state: parseState(p.State), facing: p.Facing, chat: p.Chat,
	}
}

// newParticipantID builds a LAN-unique participant id.
func newParticipantID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano()^int64(os.Getpid()))
}
