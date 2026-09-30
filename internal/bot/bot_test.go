package bot

import (
	"math/rand"
	"strings"
	"testing"

	"shooter/internal/doc"
)

// TestThinkIsDeterministicPerSeed: same seed = same decision sequence.
func TestThinkIsDeterministicPerSeed(t *testing.T) {
	d := doc.New()
	d.Load("shared buffer text\nsecond line here")
	caret := doc.Pos{Line: 0, Col: 3}

	seq := func() []Action {
		r := rand.New(rand.NewSource(42))
		out := make([]Action, 25)
		for i := range out {
			out[i] = Think(d, caret, r)
		}
		return out
	}
	a, b := seq(), seq()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("step %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// TestThinkActionsStayInBounds: every target the bot can produce exists in
// the buffer it was given.
func TestThinkActionsStayInBounds(t *testing.T) {
	d := doc.New()
	d.Load("alpha beta\ngamma\n\nlong line of words to destroy")
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 500; i++ {
		a := Think(d, doc.Pos{}, r)
		switch a.Kind {
		case ActShoot:
			if a.Pos.Line < 0 || a.Pos.Line >= d.LineCount() {
				t.Fatalf("line out of range: %+v", a)
			}
			if a.Pos.Col < 0 || a.Pos.Col >= d.RuneCount(a.Pos.Line) {
				t.Fatalf("col out of range: %+v (line has %d)",
					a, d.RuneCount(a.Pos.Line))
			}
		case ActWalk:
			p := d.Clamp(a.Pos)
			if p != a.Pos {
				t.Fatalf("walk target not clamped: %+v", a.Pos)
			}
		case ActType, ActChat, ActWord, ActLine, ActNothing:
			// always valid
		default:
			t.Fatalf("unknown action kind %d", a.Kind)
		}
	}
}

// TestThinkEmptyBufferOnlyTypes: with nothing to destroy the bot writes.
func TestThinkEmptyBufferOnlyTypes(t *testing.T) {
	d := doc.New()
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 50; i++ {
		a := Think(d, doc.Pos{}, r)
		if a.Kind != ActType {
			t.Fatalf("action = %+v, want ActType on an empty buffer", a)
		}
	}
}

// TestCheesyLinesExist: the bot must always have something rude to say,
// starting with the classic.
func TestCheesyLinesExist(t *testing.T) {
	found := false
	for _, l := range Lines {
		if strings.Contains(l, "got'em") {
			found = true
		}
		if strings.TrimSpace(l) == "" {
			t.Fatal("empty chat line")
		}
	}
	if !found {
		t.Fatal(`missing the classic "got'em"`)
	}
}

// TestDestructiveBias: the majority of decisions over a full buffer must
// be destructive (shoot/word/line) — the bot is here to break things.
func TestDestructiveBias(t *testing.T) {
	d := doc.New()
	d.Load("one two three\nfour five six\nseven eight nine")
	r := rand.New(rand.NewSource(99))
	destructive := 0
	const n = 300
	for i := 0; i < n; i++ {
		switch Think(d, doc.Pos{}, r).Kind {
		case ActShoot, ActWord, ActLine:
			destructive++
		}
	}
	if destructive < n/2 {
		t.Fatalf("destructive = %d/%d, want at least half", destructive, n)
	}
}
