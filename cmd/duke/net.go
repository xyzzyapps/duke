package main

import (
	"fmt"
	"log"
	"math"
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

// vBot is the VIRTUAL test bot: a peer simulated entirely outside the
// engine, so it can never share the human's cursor. It owns its own
// caret (cursor2), walks by easing its fractional cell, edits the buffer
// through the observed doc (its ops replicate like any client's) and
// reports presence/chat like a real peer. The human's gunman (cursor1,
// the engine) is untouched - both always move independently.
type vBot struct {
	r      *rand.Rand
	next   float64 // seconds until the next decision
	line   float64 // fractional cell position (walking eases to goal)
	col    float64
	goal   doc.Pos
	state  string // wire state name: "idle" / "walk" / "aim"
	facing int
	presT  float64
	last   netplay.Peer // last presence sent (change detection)
	id     string
}

// netCtl owns the LAN session: message pump, peer table, presence
// throttling, chat bubbles and the optional virtual test bot. A nil
// *netCtl is a valid solo-play controller (every method is nil-safe), so
// the game loop carries no mode switches.
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
	vbot         *vBot                 // virtual scripted peer (nil unless -bot)
	localChat    *render.Bubble
	disconnected bool
	g            grid.Grid // cell -> world pixel mapping for peers
}

// vbotID is the virtual bot's stable-ish participant id.
func vbotID() string {
	return fmt.Sprintf("vb-%04x", rand.New(rand.NewSource(time.Now().UnixNano())).Intn(0xffff))
}

// newNetCtl returns a controller for the given session (nil-safe in solo).
// seed > 0 makes the virtual bot deterministic (same script every run -
// used by the pixel-diff movement tests).
func newNetCtl(sess netplay.Session, host *netplay.Host, observed *netplay.ObservedDoc,
	g grid.Grid, name string, withBot bool, seed int64) *netCtl {
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
		src := time.Now().UnixNano()
		if seed != 0 {
			src = seed
		}
		nc.vbot = &vBot{
			r:      rand.New(rand.NewSource(src)),
			next:   0.8,
			state:  "idle",
			facing: 1,
			id:     vbotID(),
		}
		log.Printf("virtual bot online (cursor2 independent of the engine)")
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
	nc.localChat = &render.Bubble{Text: text, Name: nc.name}
	log.Printf("chat: %s", text)
}

// Update pumps the session: applies incoming messages, throttles outgoing
// presence, ages bubbles and advances the virtual bot. Call once per frame.
func (nc *netCtl) Update(dt float64, d doc.Document, engine engineAPI, v actions.View) {
	if nc == nil || nc.sess == nil {
		return
	}
	nc.pump(d)
	nc.presence(dt, v)
	nc.ageBubbles(dt)
	nc.vbotTick(dt, d)
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
		log.Printf("peer moves: %s L%d C%d %s facing %d",
			p.name, p.line+1, p.col+1, p.state.String(), p.facing)
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

// vbotTick advances the virtual peer once per frame: it eases its own
// fractional cell toward the goal (cursor2), then acts through the
// observed doc (its edits replicate like any client's) and reports
// presence/chat. The engine is NEVER touched, so the human's gunman in
// this window (cursor1) keeps answering to the keyboard alone.
func (nc *netCtl) vbotTick(dt float64, d doc.Document) {
	vb := nc.vbot
	if vb == nil {
		return
	}
	// Walk: move toward the goal at ~9 cells/second, flipping facing.
	if vb.line != float64(vb.goal.Line) || vb.col != float64(vb.goal.Col) {
		_, dc := float64(vb.goal.Line)-vb.line, float64(vb.goal.Col)-vb.col
		step := 9 * dt
		f := func(a, b float64) float64 {
			m := step
			if math.Abs(b-a) < m {
				m = math.Abs(b - a)
			}
			if b < a {
				m = -m
			}
			return a + m
		}
		vb.line = f(vb.line, float64(vb.goal.Line))
		vb.col = f(vb.col, float64(vb.goal.Col))
		vb.state = "walk"
		if dc > 0 {
			vb.facing = 1
		} else if dc < 0 {
			vb.facing = -1
		}
	} else {
		vb.state = "idle"
	}

	vb.next -= dt
	if vb.next <= 0 && vb.state == "idle" {
		vb.next = 0.6 + vb.r.Float64()*1.6
		pos := doc.Pos{Line: int(vb.line), Col: int(vb.col)}
		act := bot.Think(d, pos, vb.r)
		switch act.Kind {
		case bot.ActWalk:
			vb.goal = d.Clamp(act.Pos)
			log.Printf("vb walks to L%d C%d", vb.goal.Line+1, vb.goal.Col+1)
		case bot.ActType:
			for _, ch := range act.Text {
				cur := doc.Pos{Line: int(vb.goal.Line), Col: int(vb.goal.Col)}
				end := nc.observed.Insert(cur, string(ch))
				vb.goal = end
				vb.line, vb.col = float64(end.Line), float64(end.Col)
			}
			log.Printf("vb types %q", act.Text)
		case bot.ActShoot:
			nc.vbotDelete(d, doc.Pos{Line: int(vb.line), Col: int(vb.col)}, act.Pos)
			log.Printf("vb shoots L%d C%d", act.Pos.Line+1, act.Pos.Col+1)
		case bot.ActWord:
			nc.vbotWord(d, doc.Pos{Line: int(vb.line), Col: int(vb.col)}, act.Back)
			log.Printf("vb shotgun word (back=%v)", act.Back)
		case bot.ActLine:
			nc.vbotLine(d, doc.Pos{Line: int(vb.line), Col: int(vb.col)}, act.Back)
			log.Printf("vb rocket line (back=%v)", act.Back)
		case bot.ActChat:
			nc.sendChatAs(vb.id, "DUKE-BOT", act.Chat)
			log.Printf("vb chats: %q", act.Chat)
		}
	}

	// Presence: every frame while walking (visually smooth remote motion,
	// the sprite interpolates cell by cell), ~6 Hz otherwise. Only sent
	// when something actually changed.
	vb.presT += dt
	rate := 1.0 / 6
	if vb.state == "walk" {
		rate = 1.0 / 60
	}
	if vb.presT < rate {
		return
	}
	vb.presT = 0
	p := netplay.Peer{
		ID: vb.id, Name: "DUKE-BOT",
		Line: int(vb.line), Col: int(vb.col),
		State: vb.state, Facing: vb.facing,
	}
	if p.Line == vb.last.Line && p.Col == vb.last.Col &&
		p.State == vb.last.State && p.Facing == vb.last.Facing {
		return
	}
	vb.last = p
	nc.sess.Send(netplay.Msg{Kind: netplay.KindPresence, Peer: p})
}

// vbotDelete removes the rune at the target cell (ActShoot), clamped to
// the line.
func (nc *netCtl) vbotDelete(d doc.Document, base, target doc.Pos) {
	t := d.Clamp(target)
	if t.Col >= d.RuneCount(t.Line) {
		return
	}
	nc.observed.Delete(t, doc.Pos{Line: t.Line, Col: t.Col + 1})
}

// vbotWord removes the word before (back) or after the caret, staying on
// the line (ActWord).
func (nc *netCtl) vbotWord(d doc.Document, caret doc.Pos, back bool) {
	line := d.Line(caret.Line)
	from, to := caret.Col, caret.Col
	if back {
		for from > 0 && line[from-1] != ' ' && line[from-1] != '\t' {
			from--
		}
	} else {
		for to < len(line) && line[to] != ' ' && line[to] != '\t' {
			to++
		}
		for to < len(line) && (line[to] == ' ' || line[to] == '\t') {
			to++
		}
	}
	if from == to {
		return
	}
	nc.observed.Delete(doc.Pos{Line: caret.Line, Col: from}, doc.Pos{Line: caret.Line, Col: to})
}

// vbotLine removes to the end of the line (back=false) or from the line
// start (back=true) - the rocket's kill range (ActLine).
func (nc *netCtl) vbotLine(d doc.Document, caret doc.Pos, back bool) {
	end := d.RuneCount(caret.Line)
	if back {
		nc.observed.Delete(doc.Pos{Line: caret.Line, Col: 0}, doc.Pos{Line: caret.Line, Col: caret.Col})
		return
	}
	nc.observed.Delete(doc.Pos{Line: caret.Line, Col: caret.Col}, doc.Pos{Line: caret.Line, Col: end})
}

// sendChatAs broadcasts a chat bubble under an explicit identity (the
// virtual bot's own name) and shows it locally through the peer table.
func (nc *netCtl) sendChatAs(id, name, text string) {
	nc.sess.Send(netplay.Msg{
		Kind: netplay.KindChat,
		Peer: netplay.Peer{ID: id, Name: name, Chat: text},
	})
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
			Frame: int(p.animT*9) % 4, // remote walk gait (9 fps cycle)
			Name:  p.name, Chat: p.chat, ChatT: p.chatT,
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
