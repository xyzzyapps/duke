package main

import (
	"math/rand"
	"testing"

	"shooter/internal/doc"
	"shooter/internal/netplay"
)

func TestFitWindowToMonitor(t *testing.T) {
	// A big monitor: the nominal canvas is kept.
	if w, h := fitWindowToMonitor(1440, 1100, 2560, 1440); w != 1440 || h != 1100 {
		t.Fatalf("spacious monitor fit = %dx%d, want 1440x1100", w, h)
	}
	// 1080p with a taskbar: height-limited, aspect preserved.
	w, h := fitWindowToMonitor(1440, 1100, 1920, 1080)
	if w >= 1440 || h > 1080-96 {
		t.Fatalf("1080p fit = %dx%d, want both scaled down", w, h)
	}
	if a, b := float64(w)/float64(h), 1440.0/1100.0; a > b+0.01 || a < b-0.01 {
		t.Fatalf("1080p fit broke the aspect: %v vs %v", a, b)
	}
	// Wide-but-short (ultrawide): the aspect must survive.
	w2, h2 := fitWindowToMonitor(1440, 1100, 3440, 900)
	if r := float64(w2) / float64(h2); r < 1.29 || r > 1.33 {
		t.Fatalf("ultrawide fit broke the aspect: %dx%d (%v)", w2, h2, r)
	}
	// Degenerate monitor metrics: the nominal size is kept.
	if w3, h3 := fitWindowToMonitor(1440, 1100, 200, 100); w3 != 1440 || h3 != 1100 {
		t.Fatalf("degenerate screen fit = %dx%d, want nominal", w3, h3)
	}
	// Invalid monitor: unchanged.
	if w4, h4 := fitWindowToMonitor(1440, 1100, 0, 0); w4 != 1440 || h4 != 1100 {
		t.Fatalf("no monitor fit = %dx%d, want nominal", w4, h4)
	}
}

// fakeSess captures outbound messages for controller tests.
type fakeSess struct {
	sent []netplay.Msg
}

func (f *fakeSess) Send(m netplay.Msg)               { f.sent = append(f.sent, m) }
func (f *fakeSess) Incoming() <-chan netplay.Inbound { return nil }
func (f *fakeSess) Close() error                     { return nil }
func (f *fakeSess) SelfID() string                   { return "human-test" }

func TestVirtualBotIsIndependentOfTheEngine(t *testing.T) {
	// The virtual bot must never need (or touch) an engine: it owns its
	// own caret, edits through the observed doc, and broadcasts its own
	// presence - the human's cursor1 machinery is untouched.
	d := doc.New()
	d.Load("one two three\nfour five six\nseven eight nine")
	var ops []netplay.DocOp
	obs := netplay.NewObservedDoc(d, func(op netplay.DocOp) { ops = append(ops, op) })
	fake := &fakeSess{}
	nc := &netCtl{
		sess:     fake,
		observed: obs,
		selfID:   "human-test",
		vbot: &vBot{
			r:      rand.New(rand.NewSource(1)),
			next:   0,
			state:  "idle",
			facing: 1,
			id:     vbotID(),
		},
	}
	// 8 simulated seconds of ticking.
	for i := 0; i < 60*8; i++ {
		nc.vbotTick(1.0/60, d)
	}
	vb := nc.vbot
	if vb == nil {
		t.Fatal("the virtual bot must stay alive")
	}
	if len(ops) == 0 {
		t.Fatal("the virtual bot never edited the buffer")
	}
	// Its caret is its own fractional state, on the grid.
	if vb.line < 0 || vb.col < 0 {
		t.Fatalf("bot off-grid: %v %v", vb.line, vb.col)
	}
	// It broadcast its own presence with its own id.
	found := false
	for _, m := range fake.sent {
		if m.Kind == netplay.KindPresence && m.Peer.ID == vb.id {
			found = true
		}
	}
	if !found {
		t.Fatal("the virtual bot must broadcast its own presence")
	}
}
