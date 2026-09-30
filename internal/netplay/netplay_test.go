package netplay

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"time"

	"shooter/internal/doc"
)

// --- protocol ---------------------------------------------------------------

func TestEncodeDecodeRoundTrip(t *testing.T) {
	cases := []Msg{
		{Kind: KindHello, ID: "abc123", Name: "DUKE"},
		{Kind: KindWelcome, ID: "host", Text: "line one\nline two",
			Peers: []Peer{{ID: "host", Name: "DUKE", Line: 2, Col: 4, State: "idle", Facing: 1}}},
		{Kind: KindOp, Origin: "abc", Seq: 7,
			Op: &DocOp{Kind: "insert", At: doc.Pos{Line: 1, Col: 3}, Text: "hi\n"}},
		{Kind: KindPresence, Peer: Peer{ID: "x", Name: "P", Line: 9, Col: 2, State: "walk", Facing: -1}},
		{Kind: KindChat, Peer: Peer{ID: "x", Chat: `got'em "quoted" \ back`}},
		{Kind: KindLeave},
	}
	for i, want := range cases {
		var buf bytes.Buffer
		if err := Encode(&buf, want); err != nil {
			t.Fatalf("case %d encode: %v", i, err)
		}
		if !strings.HasSuffix(buf.String(), "\n") {
			t.Fatalf("case %d: message must end with newline: %q", i, buf.String())
		}
		var got Msg
		br := bufio.NewReader(&buf)
		if err := Decode(br, &got); err != nil {
			t.Fatalf("case %d decode: %v", i, err)
		}
		if got.Kind != want.Kind || got.ID != want.ID || got.Name != want.Name ||
			got.Origin != want.Origin || got.Seq != want.Seq {
			t.Fatalf("case %d: got %+v want %+v", i, got, want)
		}
		if len(got.Peers) != len(want.Peers) {
			t.Fatalf("case %d: peers = %+v", i, got.Peers)
		}
		if (got.Op == nil) != (want.Op == nil) {
			t.Fatalf("case %d: op presence mismatch", i)
		}
		if got.Op != nil && *got.Op != *want.Op {
			t.Fatalf("case %d: op %+v want %+v", i, *got.Op, *want.Op)
		}
		if got.Peer != want.Peer {
			t.Fatalf("case %d: peer %+v want %+v", i, got.Peer, want.Peer)
		}
	}
}

func TestDecodeTwoMessagesInSequence(t *testing.T) {
	var buf bytes.Buffer
	_ = Encode(&buf, Msg{Kind: KindChat, Peer: Peer{Chat: "one"}})
	_ = Encode(&buf, Msg{Kind: KindChat, Peer: Peer{Chat: "two"}})
	br := bufio.NewReader(&buf)
	for _, want := range []string{"one", "two"} {
		var m Msg
		if err := Decode(br, &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if m.Peer.Chat != want {
			t.Fatalf("chat = %q want %q", m.Peer.Chat, want)
		}
	}
}

// --- observed doc -----------------------------------------------------------

func TestObservedDocEmitsOwnEdits(t *testing.T) {
	var ops []DocOp
	o := NewObservedDoc(doc.New(), func(op DocOp) { ops = append(ops, op) })

	o.Insert(doc.Pos{Line: 0, Col: 0}, "hello world")
	o.Delete(doc.Pos{Line: 0, Col: 5}, doc.Pos{Line: 0, Col: 6})
	o.Load("a\nb")
	o.MoveLine(0, 1)

	if len(ops) != 4 {
		t.Fatalf("ops = %d, want 4: %+v", len(ops), ops)
	}
	if ops[0].Kind != "insert" || ops[0].Text != "hello world" {
		t.Fatalf("op0 = %+v", ops[0])
	}
	if ops[1].Kind != "delete" || ops[1].End != (doc.Pos{Line: 0, Col: 6}) {
		t.Fatalf("op1 = %+v", ops[1])
	}
	if ops[2].Kind != "load" || ops[2].Text != "a\nb" {
		t.Fatalf("op2 = %+v", ops[2])
	}
	if ops[3].Kind != "moveline" || ops[3].Delta != 1 {
		t.Fatalf("op3 = %+v", ops[3])
	}
}

func TestObservedDocReplaySuppressesEmission(t *testing.T) {
	var ops []DocOp
	o := NewObservedDoc(doc.New(), func(op DocOp) { ops = append(ops, op) })

	o.ApplyRemote(OpForInsert(doc.Pos{Line: 0, Col: 0}, "remote"))
	o.ApplyRemote(OpForDelete(doc.Pos{Line: 0, Col: 0}, doc.Pos{Line: 0, Col: 6}))
	o.ApplyRemote(OpForLoad("back"))

	if len(ops) != 0 {
		t.Fatalf("replay must not emit, got %+v", ops)
	}
	if o.Text() != "back" {
		t.Fatalf("text = %q", o.Text())
	}
	// A local edit after replays emits again (flag must reset).
	o.Insert(doc.Pos{Line: 0, Col: 4}, "!")
	if len(ops) != 1 {
		t.Fatalf("ops = %+v, want exactly one", ops)
	}
}

func TestObservedDocUndoEmitsNothing(t *testing.T) {
	var ops []DocOp
	o := NewObservedDoc(doc.New(), func(op DocOp) { ops = append(ops, op) })
	o.Insert(doc.Pos{Line: 0, Col: 0}, "abc")
	ops = nil
	o.Undo()
	o.Redo()
	if len(ops) != 0 {
		t.Fatalf("undo/redo must not emit (multiplayer disables them), got %+v", ops)
	}
}

// --- localhost integration --------------------------------------------------

// startHost returns a host bound to an ephemeral localhost port.
func startHost(t *testing.T) *Host {
	t.Helper()
	h, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// recv waits for one message on an incoming channel.
func recv(t *testing.T, ch <-chan Inbound) Inbound {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a message")
		return Inbound{}
	}
}

func TestClientMessageReachesHost(t *testing.T) {
	h := startHost(t)
	c, err := Dial(h.Addr(), "PLAYER", "id1")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	c.Send(Msg{Kind: KindHello, ID: "id1", Name: "PLAYER"})
	in := recv(t, h.Incoming())
	if in.Msg.Kind != KindHello || in.Msg.Name != "PLAYER" || in.From == 0 {
		t.Fatalf("inbound = %+v", in)
	}
}

func TestHostBroadcastReachesOtherClients(t *testing.T) {
	h := startHost(t)
	a, err := Dial(h.Addr(), "A", "ida")
	if err != nil {
		t.Fatalf("dial a: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	b, err := Dial(h.Addr(), "B", "idb")
	if err != nil {
		t.Fatalf("dial b: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	// Drain both hellos at the host.
	recv(t, h.Incoming())
	recv(t, h.Incoming())

	// Broadcast an op; both clients must see it.
	h.Broadcast(Msg{Kind: KindOp, Origin: "host", Seq: 1,
		Op: &DocOp{Kind: "insert", At: doc.Pos{Line: 0, Col: 0}, Text: "shared"}})
	for _, c := range []*Client{a, b} {
		in := recv(t, c.Incoming())
		if in.Msg.Kind != KindOp || in.Msg.Op == nil || in.Msg.Op.Text != "shared" {
			t.Fatalf("client got %+v", in.Msg)
		}
	}
}

func TestHostSendToRepliesToOneClient(t *testing.T) {
	h := startHost(t)
	a, _ := Dial(h.Addr(), "A", "ida")
	t.Cleanup(func() { _ = a.Close() })
	b, _ := Dial(h.Addr(), "B", "idb")
	t.Cleanup(func() { _ = b.Close() })

	inA := recv(t, h.Incoming())
	inB := recv(t, h.Incoming())

	h.SendTo(inA.From, Msg{Kind: KindWelcome, Text: "only for a"})
	if m := recv(t, a.Incoming()); m.Msg.Kind != KindWelcome {
		t.Fatalf("a got %+v", m.Msg)
	}
	// b must NOT receive it: give it a moment, then verify nothing (the
	// next message b gets is unrelated - use a presence broadcast as a
	// sentinel).
	h.Broadcast(Msg{Kind: KindPresence})
	first := recv(t, b.Incoming())
	if first.Msg.Kind == KindWelcome {
		t.Fatal("b received a's welcome")
	}
	_ = inB
}

func TestClientCloseNotifiesHostWithLeave(t *testing.T) {
	h := startHost(t)
	c, err := Dial(h.Addr(), "LEAVER", "idL")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	recv(t, h.Incoming()) // hello
	_ = c.Close()

	// The host's read loop reports a leave (or the hello reordering is
	// irrelevant - wait for a leave specifically).
	deadline := time.After(3 * time.Second)
	for {
		select {
		case in := <-h.Incoming():
			if in.Msg.Kind == KindLeave {
				return // success
			}
		case <-deadline:
			t.Fatal("no leave message after client closed")
		}
	}
}

func TestDialToDeadHostFails(t *testing.T) {
	h := startHost(t)
	addr := h.Addr()
	_ = h.Close()
	if _, err := Dial(addr, "X", "idx"); err == nil {
		t.Fatal("dialing a closed host must fail")
	}
}
