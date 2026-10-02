package actions

import (
	"strings"
	"testing"

	"shooter/internal/agent"
	"shooter/internal/doc"
	"shooter/internal/fx"
	"shooter/internal/grid"
	"shooter/internal/netplay"
)

const (
	dt    = 1.0 / 60.0
	cellW = 16
	cellH = 32
)

// testGrid matches the runtime layout assumptions.
var testGrid = grid.Grid{CellW: cellW, CellH: cellH}

// newEngine returns an engine over a loaded document.
func newEngine(text string) (Engine, doc.Document) {
	d := doc.New()
	d.Load(text)
	return New(d, testGrid), d
}

// runFor ticks the engine for the given duration and returns the last view.
func runFor(e Engine, seconds float64) View {
	var v View
	for t := 0.0; t < seconds; t += dt {
		e.Tick(dt)
		v = e.View()
	}
	return v
}

// runUntil ticks until pred is true or maxSeconds elapse.
func runUntil(e Engine, maxSeconds float64, pred func() bool) bool {
	for t := 0.0; t < maxSeconds; t += dt {
		e.Tick(dt)
		if pred() {
			return true
		}
	}
	return pred()
}

func TestTypingInsertsRunesInOrder(t *testing.T) {
	e, d := newEngine("")
	for _, r := range "hello" {
		e.TypeRune(r)
	}
	if !runUntil(e, 3, func() bool { return d.Text() == "hello" }) {
		t.Fatalf("text = %q, want hello", d.Text())
	}
	if got := e.Caret(); got != (doc.Pos{Line: 0, Col: 5}) {
		t.Fatalf("caret = %+v, want {0 5}", got)
	}
}

func TestTypingInsertsInstantlyWithSound(t *testing.T) {
	// Typing has no flight animation: the rune locks into the buffer on
	// the first tick and its arrival is heard (the stamp sound). No FX.
	e, d := newEngine("")
	e.TypeRune('x')
	stamp := false
	ok := runUntil(e, 1, func() bool {
		v := e.View()
		for _, s := range v.Sounds {
			if s == fx.SoundStamp {
				stamp = true
			}
		}
		if len(v.FX) != 0 {
			t.Fatalf("unexpected FX = %v (typing must not animate)", v.FX)
		}
		return d.Text() == "x" && stamp
	})
	if !ok {
		t.Fatalf("text = %q, stamp heard: %v", d.Text(), stamp)
	}
	if got := e.Caret(); got != (doc.Pos{Line: 0, Col: 1}) {
		t.Fatalf("caret = %+v, want {0 1}", got)
	}
}

func TestBackspaceShootsGlyphBehindCaret(t *testing.T) {
	e, d := newEngine("ab")
	e.WalkTo(doc.Pos{Line: 0, Col: 2})
	runFor(e, 0.5) // let him arrive at the caret
	// turn-first: the first backwards press only turns him left
	// turn-first: the first backwards press only turns him left
	e.Shoot(true)
	e.Shoot(true)
	if !runUntil(e, 2, func() bool { return d.Text() == "a" }) {
		t.Fatalf("text = %q, want a", d.Text())
	}
	if got := e.Caret(); got != (doc.Pos{Line: 0, Col: 1}) {
		t.Fatalf("caret = %+v, want {0 1}", got)
	}

}

func TestBackspaceEmitsShatterFXWithRunes(t *testing.T) {
	e, d := newEngine("ab")
	e.WalkTo(doc.Pos{Line: 0, Col: 2})
	runFor(e, 0.5)
	// turn-first: the first backwards press only turns him left
	// turn-first: the first backwards press only turns him left
	e.Shoot(true)
	e.Shoot(true)
	var shatter *fx.Effect
	runUntil(e, 2, func() bool {
		if d.Text() != "a" {
			return false
		}
		// Drain FX each tick to catch the shatter.
		for _, f := range e.View().FX {
			if f.Kind == fx.Shatter {
				cp := f
				shatter = &cp
			}
		}
		return shatter != nil
	})
	if shatter == nil {
		t.Fatal("no shatter FX observed")
	}
	if shatter.Runes != "b" {
		t.Fatalf("shattered runes = %q, want b", shatter.Runes)
	}
}

func TestForwardShootDeletesGlyphAtCaret(t *testing.T) {
	e, d := newEngine("ab")
	e.Shoot(false)
	if !runUntil(e, 2, func() bool { return d.Text() == "b" }) {
		t.Fatalf("text = %q, want b", d.Text())
	}
	if got := e.Caret(); got != (doc.Pos{Line: 0, Col: 0}) {
		t.Fatalf("caret = %+v, want {0 0}", got)
	}
}

func TestBackspaceAtBufferStartReportsStatus(t *testing.T) {
	e, d := newEngine("")
	// turn-first: the first backwards press only turns him left
	// turn-first: the first backwards press only turns him left
	e.Shoot(true)
	e.Shoot(true)
	runFor(e, 0.3)
	if d.Text() != "" {
		t.Fatalf("text changed: %q", d.Text())
	}
	if !strings.Contains(e.Status(), "NOTHING TO SHOOT") {
		t.Fatalf("status = %q", e.Status())
	}
}

func TestBackspaceJoinsLines(t *testing.T) {
	e, d := newEngine("ab\ncd")
	e.WalkTo(doc.Pos{Line: 1, Col: 0})
	runFor(e, 0.5)
	// turn-first: the first backwards press only turns him left
	// turn-first: the first backwards press only turns him left
	e.Shoot(true)
	e.Shoot(true)
	if !runUntil(e, 2, func() bool { return d.Text() == "abcd" }) {
		t.Fatalf("text = %q, want abcd", d.Text())
	}
	if got := e.Caret(); got != (doc.Pos{Line: 0, Col: 2}) {
		t.Fatalf("caret = %+v, want {0 2}", got)
	}
}

func TestForwardShootAtEndOfLineJoinsNext(t *testing.T) {
	e, d := newEngine("ab\ncd")
	e.WalkTo(doc.Pos{Line: 0, Col: 2})
	runFor(e, 0.5)
	e.Shoot(false)
	if !runUntil(e, 2, func() bool { return d.Text() == "abcd" }) {
		t.Fatalf("text = %q, want abcd", d.Text())
	}
}

func TestMoveLineSwapsAndAnimates(t *testing.T) {
	e, d := newEngine("a\nb\nc")
	e.MoveLine(+1)
	// The document swaps immediately, the animation follows.
	e.Tick(dt)
	if d.Text() != "b\na\nc" {
		t.Fatalf("text = %q right after move", d.Text())
	}
	if got := e.Caret(); got != (doc.Pos{Line: 1, Col: 0}) {
		t.Fatalf("caret = %+v, want {1 0}", got)
	}
	// Animation must be visible early on...
	v := runFor(e, 0.05)
	if len(v.Swaps) != 1 {
		t.Fatalf("swaps = %+v, want 1 during animation", v.Swaps)
	}
	// ...and gone once complete.
	v = runFor(e, 0.5)
	if len(v.Swaps) != 0 {
		t.Fatalf("swaps = %+v, want none after animation", v.Swaps)
	}
	// The agent must end up on the caret cell.
	x, y := testGrid.AgentOrigin(doc.Pos{Line: 1, Col: 0}, agent.SpriteH)
	v = runFor(e, 1.0)
	if v.Agent.X != x || v.Agent.Y != y {
		t.Fatalf("agent at (%v,%v), want (%v,%v)", v.Agent.X, v.Agent.Y, x, y)
	}
}

func TestMoveLineAtEdgeReportsStatus(t *testing.T) {
	e, d := newEngine("only")
	e.MoveLine(-1)
	runFor(e, 0.3)
	if d.Text() != "only" {
		t.Fatalf("text = %q", d.Text())
	}
	if !strings.Contains(e.Status(), "EDGE") {
		t.Fatalf("status = %q", e.Status())
	}
}

func TestUndoRedoTyping(t *testing.T) {
	e, d := newEngine("")
	e.TypeRune('z')
	runUntil(e, 2, func() bool { return d.Text() == "z" })
	e.Undo()
	runUntil(e, 2, func() bool { return d.Text() == "" })
	e.Redo()
	runUntil(e, 2, func() bool { return d.Text() == "z" })
	if d.Text() != "z" {
		t.Fatalf("text = %q", d.Text())
	}
}

func TestUndoWithoutHistoryReportsStatus(t *testing.T) {
	e, _ := newEngine("")
	e.Undo()
	runFor(e, 0.3)
	if !strings.Contains(e.Status(), "NOTHING TO UNDO") {
		t.Fatalf("status = %q", e.Status())
	}
}

func TestWalkToMovesCaretAndAgent(t *testing.T) {
	e, d := newEngine("hello\nworld")
	e.WalkTo(doc.Pos{Line: 1, Col: 3})
	if got := e.Caret(); got != (doc.Pos{Line: 1, Col: 3}) {
		t.Fatalf("caret = %+v immediately after WalkTo", got)
	}
	wantX, wantY := testGrid.AgentOrigin(doc.Pos{Line: 1, Col: 3}, agent.SpriteH)
	v := runUntil(e, 3, func() bool {
		s := e.View().Agent
		return s.X == wantX && s.Y == wantY
	})
	if !v {
		s := e.View().Agent
		t.Fatalf("agent at (%v,%v), want (%v,%v)", s.X, s.Y, wantX, wantY)
	}
	_ = d
}

func TestClickThenTypeLandsAtClickedCell(t *testing.T) {
	e, d := newEngine("........")
	e.WalkTo(doc.Pos{Line: 0, Col: 3})
	e.TypeRune('!')
	if !runUntil(e, 3, func() bool { return d.Text() == "...!....." }) {
		t.Fatalf("text = %q", d.Text())
	}
}

func TestCommandOrderIsFIFO(t *testing.T) {
	e, d := newEngine("")
	e.TypeRune('a')
	e.TypeRune('b')
	// turn-first: the first backwards press only turns him left
	// turn-first: the first backwards press only turns him left
	e.Shoot(true) // should fire only after both letters landed
	e.Shoot(true) // should fire only after both letters landed
	e.TypeRune('c')
	if !runUntil(e, 3, func() bool { return d.Text() == "ac" }) {
		t.Fatalf("text = %q, want ac (ab shot away, then c)", d.Text())
	}
}

func TestCancelDropsQueuedWork(t *testing.T) {
	e, d := newEngine("")
	for _, r := range "abcdef" {
		e.TypeRune(r)
	}
	e.Cancel()
	runFor(e, 1.0)
	if d.Text() != "" {
		t.Fatalf("text = %q, want empty after Cancel", d.Text())
	}
}

func TestCelebratePlaysWinPose(t *testing.T) {
	e, _ := newEngine("")
	e.Celebrate()
	var v View
	runUntil(e, 1, func() bool {
		v = e.View()
		return v.Agent.State == agent.StateWin
	})
	if v.Agent.State != agent.StateWin {
		t.Fatalf("state = %v, want win", v.Agent.State)
	}
	runFor(e, 1.0)
	if e.View().Agent.State == agent.StateWin {
		t.Fatal("win pose should end")
	}
}

func TestStatusExpires(t *testing.T) {
	e, _ := newEngine("")
	e.SetStatus("HELLO")
	if e.Status() != "HELLO" {
		t.Fatalf("status = %q", e.Status())
	}
	runFor(e, 3)
	if e.Status() != "" {
		t.Fatalf("status = %q after expiry", e.Status())
	}
}

func TestViewDrainsFXTwice(t *testing.T) {
	e, _ := newEngine("")
	// Celebrate enqueues a pose whose start emits a SaveFlare effect.
	e.Celebrate()
	e.Tick(dt)
	first := e.View()
	if len(first.FX) != 1 || first.FX[0].Kind != fx.SaveFlare {
		t.Fatalf("first FX = %+v, want one SaveFlare", first.FX)
	}
	second := e.View()
	if len(second.FX) != 0 {
		t.Fatalf("second FX = %+v, want drained empty", second.FX)
	}
}

func TestAutoFireBackpressure(t *testing.T) {
	e, d := newEngine(strings.Repeat("x", 200))
	e.WalkTo(doc.Pos{Line: 0, Col: 200})
	runFor(e, 2)
	// Hammer shoot far beyond the cap; queue must stay bounded.
	for i := 0; i < 50; i++ {
		e.Shoot(true)
	}
	if n := e.(*engine).countShots(); n > maxShootQ {
		t.Fatalf("queued shots = %d, want <= %d", n, maxShootQ)
	}
	runFor(e, 1.0)
	if d.RuneCount(0) >= 200 {
		t.Fatal("shots should have deleted some text")
	}
}

func TestAdjacentBackspaceIsMelee(t *testing.T) {
	e, d := newEngine("ab")
	e.WalkTo(doc.Pos{Line: 0, Col: 2})
	runFor(e, 0.5)
	// turn-first: the first backwards press only turns him left
	// turn-first: the first backwards press only turns him left
	e.Shoot(true)
	e.Shoot(true)
	sawSlash, sawBullet := false, false
	ok := runUntil(e, 2, func() bool {
		v := e.View()
		if v.Agent.State == agent.StateSlash {
			sawSlash = true
		}
		if len(v.Bullets) > 0 {
			sawBullet = true
		}
		return d.Text() == "a"
	})
	if !ok {
		t.Fatalf("text = %q, want a", d.Text())
	}
	if !sawSlash {
		t.Fatal("an adjacent target must be a katana swing (not the pistol)")
	}
	if sawBullet {
		t.Fatal("a katana swing must not fire the gun")
	}
}

func TestFarJoinUsesTheGun(t *testing.T) {
	// Caret at the start of line 1 with a long line above: the backspace
	// target is ~40 cells away, so he must shoot it.
	e, d := newEngine(strings.Repeat("x", 40) + "\n")
	e.WalkTo(doc.Pos{Line: 1, Col: 0})
	runFor(e, 1.0)
	// turn-first: the first backwards press only turns him left
	// turn-first: the first backwards press only turns him left
	e.Shoot(true)
	e.Shoot(true)
	sawBullet := false
	ok := runUntil(e, 3, func() bool {
		if len(e.View().Bullets) > 0 {
			sawBullet = true
		}
		return d.LineCount() == 1
	})
	if !ok {
		t.Fatalf("lines = %d, want 1 after the join", d.LineCount())
	}
	if !sawBullet {
		t.Fatal("a distant target must be shot with the gun")
	}
	if d.RuneCount(0) != 40 {
		t.Fatalf("joined line has %d runes, want 40", d.RuneCount(0))
	}
}

func TestShootAtDistantGlyphUsesGunAndKeepsCaret(t *testing.T) {
	e, d := newEngine("hello world\nsecond")
	e.ShootAt(doc.Pos{Line: 0, Col: 6}) // the 'w', six cells away
	sawBullet := false
	ok := runUntil(e, 3, func() bool {
		if len(e.View().Bullets) > 0 {
			sawBullet = true
		}
		return d.Text() == "hello orld\nsecond"
	})
	if !ok {
		t.Fatalf("text = %q", d.Text())
	}
	if !sawBullet {
		t.Fatal("a distant right-click must use the gun")
	}
	if e.Caret() != (doc.Pos{Line: 0, Col: 0}) {
		t.Fatalf("caret = %+v, want {0 0} (he shoots from where he stands)", e.Caret())
	}
}

func TestShootAtAdjacentGlyphUsesFists(t *testing.T) {
	e, d := newEngine("abc")
	e.ShootAt(doc.Pos{Line: 0, Col: 1}) // one cell from caret (0,0)
	sawSlash, sawBullet := false, false
	ok := runUntil(e, 2, func() bool {
		v := e.View()
		if v.Agent.State == agent.StateSlash {
			sawSlash = true
		}
		if len(v.Bullets) > 0 {
			sawBullet = true
		}
		return d.Text() == "ac"
	})
	if !ok {
		t.Fatalf("text = %q, want ac", d.Text())
	}
	if !sawSlash || sawBullet {
		t.Fatalf("adjacent target: slash=%v bullet=%v, want slash only", sawSlash, sawBullet)
	}
	if e.Caret() != (doc.Pos{Line: 0, Col: 0}) {
		t.Fatalf("caret = %+v, want {0 0}", e.Caret())
	}
}

func TestShootAtLastCellReportsStatus(t *testing.T) {
	e, d := newEngine("abc") // no line after the last cell
	e.ShootAt(doc.Pos{Line: 0, Col: 3})
	runFor(e, 0.3)
	if d.Text() != "abc" {
		t.Fatalf("text = %q, want unchanged", d.Text())
	}
	if !strings.Contains(e.Status(), "NOTHING TO SHOOT") {
		t.Fatalf("status = %q", e.Status())
	}
}

func TestShiftCaretAfterDeletes(t *testing.T) {
	single := func(from, to doc.Pos) func(doc.Pos) doc.Pos {
		return func(c doc.Pos) doc.Pos { return shiftCaret(c, from, to) }
	}
	s := single(doc.Pos{Line: 1, Col: 2}, doc.Pos{Line: 1, Col: 5})
	// After the range: shifts left by the deleted width.
	if got := s(doc.Pos{Line: 1, Col: 9}); got != (doc.Pos{Line: 1, Col: 6}) {
		t.Fatalf("after range: %+v", got)
	}
	// Before the range: unchanged.
	if got := s(doc.Pos{Line: 1, Col: 1}); got != (doc.Pos{Line: 1, Col: 1}) {
		t.Fatalf("before range: %+v", got)
	}
	// Inside the range: collapses to the join point.
	if got := s(doc.Pos{Line: 1, Col: 3}); got != (doc.Pos{Line: 1, Col: 2}) {
		t.Fatalf("inside range: %+v", got)
	}
	// Multi-line join: a caret on the removed tail line collapses into
	// the join point; lines below shift up.
	from, to := doc.Pos{Line: 1, Col: 2}, doc.Pos{Line: 2, Col: 4}
	if got := shiftCaret(doc.Pos{Line: 2, Col: 6}, from, to); got != (doc.Pos{Line: 1, Col: 4}) {
		t.Fatalf("on tail line: %+v, want {1 4}", got)
	}
	if got := shiftCaret(doc.Pos{Line: 3, Col: 1}, from, to); got != (doc.Pos{Line: 2, Col: 1}) {
		t.Fatalf("below join: %+v, want {2 1}", got)
	}
}

func TestWordRangeHelpers(t *testing.T) {
	d := doc.New()
	d.Load("kill one  two")
	// Back from end of "one": takes the word (and nothing after it).
	from, to, ok := wordRange(d, doc.Pos{Line: 0, Col: 8}, true)
	if !ok || from != (doc.Pos{Line: 0, Col: 5}) || to != (doc.Pos{Line: 0, Col: 8}) {
		t.Fatalf("back: from=%+v to=%+v ok=%v", from, to, ok)
	}
	// Forward from start: the word + its trailing blanks ("kill ").
	from, to, ok = wordRange(d, doc.Pos{Line: 0, Col: 0}, false)
	if !ok || from != (doc.Pos{Line: 0, Col: 0}) || to != (doc.Pos{Line: 0, Col: 5}) {
		t.Fatalf("fwd: from=%+v to=%+v ok=%v", from, to, ok)
	}
	// Line start / line end have nothing on that side.
	if _, _, ok := wordRange(d, doc.Pos{Line: 0, Col: 0}, true); ok {
		t.Fatal("word back at line start must fail")
	}
	if _, _, ok := wordRange(d, doc.Pos{Line: 0, Col: 13}, false); ok {
		t.Fatal("word fwd at line end must fail")
	}
}

func TestKillRangeHelpers(t *testing.T) {
	d := doc.New()
	d.Load("alpha\nbeta")
	// Ctrl+K mid-line: to end of line.
	from, to, ok := killRange(d, doc.Pos{Line: 0, Col: 3}, false)
	if !ok || from != (doc.Pos{Line: 0, Col: 3}) || to != (doc.Pos{Line: 0, Col: 5}) {
		t.Fatalf("k: from=%+v to=%+v ok=%v", from, to, ok)
	}
	// Ctrl+K at EOL: kills the newline (joins).
	from, to, ok = killRange(d, doc.Pos{Line: 0, Col: 5}, false)
	if !ok || to != (doc.Pos{Line: 1, Col: 0}) {
		t.Fatalf("k at eol: to=%+v ok=%v", to, ok)
	}
	// Ctrl+U: from line start to caret.
	from, to, ok = killRange(d, doc.Pos{Line: 0, Col: 3}, true)
	if !ok || from != (doc.Pos{Line: 0, Col: 0}) || to != (doc.Pos{Line: 0, Col: 3}) {
		t.Fatalf("u: from=%+v to=%+v ok=%v", from, to, ok)
	}
	// Ctrl+U at line start: nothing to kill.
	if _, _, ok := killRange(d, doc.Pos{Line: 0, Col: 0}, true); ok {
		t.Fatal("kill back at line start must fail")
	}
	// Ctrl+K on the last line at EOL: nothing to kill.
	if _, _, ok := killRange(d, doc.Pos{Line: 1, Col: 4}, false); ok {
		t.Fatal("kill fwd at buffer end must fail")
	}
}

func TestShootWordFiresTheShotgun(t *testing.T) {
	e, d := newEngine("kill one two")
	e.WalkTo(doc.Pos{Line: 0, Col: 8}) // end of "one"
	runFor(e, 0.3)
	e.ShootWord(true) // turn-first: press 1 turns him left, press 2 fires
	e.ShootWord(true)
	spread, boom, done := false, false, false
	runUntil(e, 3, func() bool {
		v := e.View()
		for _, s := range v.Sounds {
			if s == fx.SoundShotgun {
				boom = true
			}
		}
		for _, b := range v.Bullets {
			if b.Spread {
				spread = true
			}
		}
		if d.Text() == "kill  two" {
			done = true
		}
		return done
	})
	if !done {
		t.Fatalf("text = %q, want %q", d.Text(), "kill  two")
	}
	if !spread {
		t.Fatal("shotgun must fire a pellet spread")
	}
	if !boom {
		t.Fatal("shotgun must cue SoundShotgun")
	}
}

func TestKillLineFiresTheRocket(t *testing.T) {
	e, d := newEngine("alpha\nbeta")
	e.WalkTo(doc.Pos{Line: 0, Col: 3})
	runFor(e, 0.3)
	e.KillLine(false) // emacs ctrl+k: to end of line
	rocket, blast, done := false, false, false
	runUntil(e, 3, func() bool {
		v := e.View()
		for _, s := range v.Sounds {
			if s == fx.SoundRocket {
				rocket = true
			}
		}
		for _, b := range v.Bullets {
			if b.Rocket {
				rocket = true
			}
		}
		for _, f := range v.FX {
			if f.Kind == fx.Explosion {
				blast = true
			}
		}
		if d.Text() == "alp\nbeta" {
			done = true
		}
		return done
	})
	if !done {
		t.Fatalf("text = %q, want %q", d.Text(), "alp\nbeta")
	}
	if !rocket {
		t.Fatal("rocket launcher round missing")
	}
	if !blast {
		t.Fatal("rocket impact must explode")
	}
}

func TestKillLineAtEOLJoinsLines(t *testing.T) {
	e, d := newEngine("alpha\nbeta")
	e.WalkTo(doc.Pos{Line: 0, Col: 5})
	runFor(e, 0.3)
	e.KillLine(false)
	if !runUntil(e, 3, func() bool { return d.LineCount() == 1 }) {
		t.Fatalf("lines = %d, want 1 (newline killed)", d.LineCount())
	}
	if d.Text() != "alphabeta" {
		t.Fatalf("text = %q", d.Text())
	}
}

func TestKatanaReachBoundary(t *testing.T) {
	// Four cells away: the giant blade still reaches (no bullet).
	e, d := newEngine("abcdef")
	e.ShootAt(doc.Pos{Line: 0, Col: 4})
	sawSlash, sawBullet := false, false
	runUntil(e, 2, func() bool {
		v := e.View()
		if v.Agent.State == agent.StateSlash {
			sawSlash = true
		}
		if len(v.Bullets) > 0 {
			sawBullet = true
		}
		return d.Text() == "abcdf"
	})
	if !sawSlash || sawBullet {
		t.Fatalf("reach 4: slash=%v bullet=%v, want slash only", sawSlash, sawBullet)
	}
	// Five cells (fresh buffer): the pistol comes out.
	e2, d2 := newEngine("abcdefgh")
	e2.ShootAt(doc.Pos{Line: 0, Col: 5})
	sawSlash, sawBullet = false, false
	runUntil(e2, 2, func() bool {
		v := e2.View()
		if v.Agent.State == agent.StateSlash {
			sawSlash = true
		}
		if len(v.Bullets) > 0 {
			sawBullet = true
		}
		return d2.Text() == "abcdegh"
	})
	if sawSlash || !sawBullet {
		t.Fatalf("reach 5: slash=%v bullet=%v, want bullet only", sawSlash, sawBullet)
	}
}

func TestSoundCuesPerAction(t *testing.T) {
	// Typing cues a stamp, the katana cues a whoosh, save cues a chime.
	e, _ := newEngine("ab")
	e.TypeRune('x')
	stamp := false
	runUntil(e, 2, func() bool {
		for _, s := range e.View().Sounds {
			if s == fx.SoundStamp {
				stamp = true
			}
		}
		return stamp
	})
	if !stamp {
		t.Fatal("letter landing must cue SoundStamp")
	}
	e.Celebrate()
	chime := false
	runUntil(e, 2, func() bool {
		for _, s := range e.View().Sounds {
			if s == fx.SoundSave {
				chime = true
			}
		}
		return chime
	})
	if !chime {
		t.Fatal("celebration must cue SoundSave")
	}
}

func TestSwapDocumentRestoresCaretAndDropsWork(t *testing.T) {
	e, d1 := newEngine("first buffer")
	e.TypeRune('x') // queued, not yet started

	d2 := doc.New()
	d2.Load("second buffer")
	e.SwapDocument(d2, doc.Pos{Line: 0, Col: 6})

	if e.Caret() != (doc.Pos{Line: 0, Col: 6}) {
		t.Fatalf("caret = %+v, want {0 6}", e.Caret())
	}
	// The agent parks on the restored caret (cell 6 of the test grid).
	wantX := float64(6 * 16)
	v := runFor(e, 0.5)
	if v.Agent.X != wantX {
		t.Fatalf("agent x = %v, want %v", v.Agent.X, wantX)
	}
	// The queued letter died with the swap: neither buffer changed.
	if d1.Text() != "first buffer" {
		t.Fatalf("old buffer = %q, want untouched", d1.Text())
	}
	if d2.Text() != "second buffer" {
		t.Fatalf("new buffer = %q, want untouched", d2.Text())
	}
}

func TestBackspaceSwingsLeftOnlyWhenStrikingBackward(t *testing.T) {
	// Backspace at a target to the left: the one time he faces left.
	e, d := newEngine("abcdef")
	e.WalkTo(doc.Pos{Line: 0, Col: 3})
	runFor(e, 0.5)
	// turn-first: the first backwards press only turns him left
	// turn-first: the first backwards press only turns him left
	e.Shoot(true)
	e.Shoot(true)
	facing := 1
	ok := runUntil(e, 2, func() bool {
		if v := e.View(); v.Agent.State == agent.StateSlash {
			facing = v.Agent.Facing
		}
		return d.Text() == "abdef"
	})
	if !ok {
		t.Fatalf("text = %q", d.Text())
	}
	if facing != -1 {
		t.Fatalf("facing = %d during backward strike, want -1", facing)
	}
}

func TestStrikesFaceTowardTheirTarget(t *testing.T) {
	// Forward delete at the caret cell (target == base): strikes forward,
	// the way the turn-first model faced him.
	e, d := newEngine("abcdef")
	e.Shoot(false)
	facing := 0
	runUntil(e, 2, func() bool {
		if v := e.View(); v.Agent.State == agent.StateSlash {
			facing = v.Agent.Facing
		}
		return d.Text() == "bcdef"
	})
	if facing != 1 {
		t.Fatalf("facing = %d on own-cell strike, want 1 (forward)", facing)
	}

	// Right-click on a glyph LEFT of the caret: faces left (toward it).
	e2, d2 := newEngine("abcdef")
	e2.WalkTo(doc.Pos{Line: 0, Col: 4})
	runFor(e2, 0.5)
	e2.ShootAt(doc.Pos{Line: 0, Col: 0})
	facing = 1
	runUntil(e2, 2, func() bool {
		if v := e2.View(); v.Agent.State == agent.StateSlash {
			facing = v.Agent.Facing
		}
		return d2.Text() == "bcdef"
	})
	if facing != -1 {
		t.Fatalf("facing = %d on explicit left target, want -1", facing)
	}

	// Right-click on a glyph to the RIGHT: faces right.
	e3, d3 := newEngine("abcdef")
	e3.WalkTo(doc.Pos{Line: 0, Col: 1})
	runFor(e3, 0.5)
	e3.ShootAt(doc.Pos{Line: 0, Col: 4})
	facing = -1
	runUntil(e3, 2, func() bool {
		if v := e3.View(); v.Agent.State == agent.StateSlash {
			facing = v.Agent.Facing
		}
		return d3.Text() == "abcef"
	})
	if facing != 1 {
		t.Fatalf("facing = %d on explicit right target, want 1", facing)
	}
}

func TestHeavyWeaponsFaceTheirAim(t *testing.T) {
	// Ctrl+U aims toward the line start (left): faces left.
	e, d := newEngine("alpha beta\ngamma")
	e.WalkTo(doc.Pos{Line: 0, Col: 10})
	runFor(e, 0.5)
	e.KillLine(true) // turn-first: press 1 turns him left, press 2 fires
	e.KillLine(true)
	facing := 1
	runUntil(e, 3, func() bool {
		v := e.View()
		if v.Agent.State == agent.StateAim || v.Agent.State == agent.StateFire {
			facing = v.Agent.Facing
		}
		return d.Text() == "\ngamma" // killed back to line start
	})
	if facing != -1 {
		t.Fatalf("facing = %d aiming left, want -1", facing)
	}

	// Ctrl+K kills to end of line (right): faces right.
	e2, d2 := newEngine("alpha\ngamma")
	e2.WalkTo(doc.Pos{Line: 0, Col: 2})
	runFor(e2, 0.5)
	e2.KillLine(false)
	facing = -1
	runUntil(e2, 3, func() bool {
		v := e2.View()
		if v.Agent.State == agent.StateAim || v.Agent.State == agent.StateFire {
			facing = v.Agent.Facing
		}
		return d2.Text() != "alpha\ngamma"
	})
	if facing != 1 {
		t.Fatalf("facing = %d aiming right, want 1", facing)
	}
}

func TestWalkingReflectsDirection(t *testing.T) {
	e, _ := newEngine("hello world")
	e.WalkTo(doc.Pos{Line: 0, Col: 9})
	runFor(e, 1.0)

	// Walking left: he faces left mid-walk.
	e.WalkTo(doc.Pos{Line: 0, Col: 0})
	sawLeft := false
	runUntil(e, 2, func() bool {
		v := e.View()
		if v.Agent.State == agent.StateWalk && v.Agent.Facing == -1 {
			sawLeft = true
		}
		return v.Agent.X == 0
	})
	if !sawLeft {
		t.Fatal("walking left must face left")
	}

	// Walking right again: he flips back to right.
	e.WalkTo(doc.Pos{Line: 0, Col: 9})
	sawRight := false
	runUntil(e, 2, func() bool {
		v := e.View()
		if v.Agent.State == agent.StateWalk && v.Agent.Facing == 1 {
			sawRight = true
		}
		return v.Agent.X == 9*16 // test grid cellW = 16
	})
	if !sawRight {
		t.Fatal("walking right must face right")
	}
}

// --- turn-first input model -------------------------------------------------

func TestFirstBackspaceTurnsThenSecondDeletes(t *testing.T) {
	e, d := newEngine("ab")
	e.WalkTo(doc.Pos{Line: 0, Col: 2})
	runFor(e, 0.5)

	// Press 1: only a turn to the left - no deletion, no cooldown.
	e.Shoot(true)
	runFor(e, 0.3)
	if d.Text() != "ab" {
		t.Fatalf("text changed on the turn press: %q", d.Text())
	}
	if v := e.View(); v.Agent.Facing != -1 {
		t.Fatalf("facing = %d after first backspace, want -1", v.Agent.Facing)
	}
	if !strings.Contains(e.Status(), "FACING LEFT") {
		t.Fatalf("status = %q, want the turn notice", e.Status())
	}

	// Press 2: the katana deletes the glyph behind.
	e.Shoot(true)
	if !runUntil(e, 2, func() bool { return d.Text() == "a" }) {
		t.Fatalf("text = %q after the second press, want a", d.Text())
	}
}

func TestStepTurnsThenWalksAndWraps(t *testing.T) {
	e, _ := newEngine("ab\ncd")
	e.WalkTo(doc.Pos{Line: 0, Col: 1})
	runFor(e, 0.5)

	// Left arrow: first press turns him, nothing moves.
	e.Step(-1)
	runFor(e, 0.3)
	if c := e.Caret(); c != (doc.Pos{Line: 0, Col: 1}) {
		t.Fatalf("caret moved on the turn press: %+v", c)
	}
	if v := e.View(); v.Agent.Facing != -1 {
		t.Fatalf("facing = %d, want -1 after the turn", v.Agent.Facing)
	}

	// Second left press walks left.
	e.Step(-1)
	if !runUntil(e, 2, func() bool { return e.Caret() == (doc.Pos{Line: 0, Col: 0}) }) {
		t.Fatalf("caret = %+v, want {0 0}", e.Caret())
	}

	// Right arrow: turn-first again, then wrap right at EOL -> next line.
	e.Step(1) // turn
	e.Step(1) // walk to col 1
	e.Step(1) // walk to col 2 (EOL)
	runFor(e, 2.0)
	e.Step(1) // facing is right: wraps to the next line start
	if !runUntil(e, 3, func() bool { return e.Caret() == (doc.Pos{Line: 1, Col: 0}) }) {
		t.Fatalf("caret = %+v, want {1 0} after the wrap", e.Caret())
	}

	// Left at col 0 wraps to the previous line end.
	e2, d2 := newEngine("ab\ncd")
	e2.WalkTo(doc.Pos{Line: 1, Col: 0})
	runFor(e2, 0.5)
	e2.Step(-2) // first press turns left
	e2.Step(-2) // second press walks left: wraps to the previous line end
	if !runUntil(e2, 2, func() bool { return e2.Caret() == (doc.Pos{Line: 0, Col: 2}) }) {
		t.Fatalf("caret = %+v, want {0 2} after the left wrap", e2.Caret())
	}
	_ = d2
}

func TestDeleteActsWithoutTurnWhenAlreadyFacingRight(t *testing.T) {
	e, d := newEngine("ab")
	// Default facing is right: Delete fires immediately.
	e.Shoot(false)
	if !runUntil(e, 2, func() bool { return d.Text() == "b" }) {
		t.Fatalf("text = %q, want b", d.Text())
	}
}

// --- strike-facing (the turn-first model decides the blade's way) ----------

func TestDeleteSlashFacesForward(t *testing.T) {
	// Delete on the glyph AT the caret: the turn-first model faced him
	// right, so the katana must swing right (target == base used to flip
	// him left instead).
	e, d := newEngine("ab")
	e.Shoot(false) // already facing right: acts immediately
	facing := 0
	ok := runUntil(e, 2, func() bool {
		if v := e.View(); v.Agent.State == agent.StateSlash {
			facing = v.Agent.Facing
		}
		return d.Text() == "b" // the 'a' at his feet is struck
	})
	if !ok {
		t.Fatalf("text = %q, want b", d.Text())
	}
	if facing != 1 {
		t.Fatalf("slash facing = %d, want 1 (forward)", facing)
	}
}

func TestForwardJoinUsesThePistol(t *testing.T) {
	// Delete at end of line joins the next line: the newline breaks on
	// the NEXT row, so the katana must not swing - the pistol takes it.
	e, d := newEngine("ab\ncd")
	e.WalkTo(doc.Pos{Line: 0, Col: 2})
	runFor(e, 0.5)
	e.Shoot(false)
	sawSlash, sawAim := false, false
	ok := runUntil(e, 2, func() bool {
		v := e.View()
		switch v.Agent.State {
		case agent.StateSlash:
			sawSlash = true
		case agent.StateAim, agent.StateFire:
			sawAim = true
		}
		return d.Text() == "abcd"
	})
	if !ok {
		t.Fatalf("text = %q, want abcd", d.Text())
	}
	if sawSlash {
		t.Fatal("a cross-line join must not use the katana")
	}
	if !sawAim {
		t.Fatal("the join must be shot with the pistol")
	}
}

func TestVerticalTargetUsesThePistol(t *testing.T) {
	// Right-click a glyph on another line within katana reach: the blade
	// only strikes along his own row.
	e, d := newEngine("abc\ndef")
	e.WalkTo(doc.Pos{Line: 0, Col: 1})
	runFor(e, 0.5)
	e.ShootAt(doc.Pos{Line: 1, Col: 1})
	sawSlash := false
	ok := runUntil(e, 2, func() bool {
		if v := e.View(); v.Agent.State == agent.StateSlash {
			sawSlash = true
		}
		return d.Text() == "abc\ndf"
	})
	if !ok {
		t.Fatalf("text = %q, want abc\\ndf", d.Text())
	}
	if sawSlash {
		t.Fatal("a vertical target must not use the katana")
	}
}

func TestExplicitOwnCellShotKeepsFacing(t *testing.T) {
	// A right-click on the glyph the gunman stands on has no geometric
	// direction: he strikes the way he is already facing.
	e, _ := newEngine("abc")
	e.WalkTo(doc.Pos{Line: 0, Col: 1})
	runFor(e, 0.5)
	e.Shoot(true) // turn left first
	runFor(e, 0.3)
	e.ShootAt(doc.Pos{Line: 0, Col: 1}) // own cell
	facing := 0
	ok := runUntil(e, 2, func() bool {
		if v := e.View(); v.Agent.State == agent.StateSlash {
			facing = v.Agent.Facing
			return true
		}
		return false
	})
	if !ok || facing != -1 {
		t.Fatalf("own-cell slash facing = %d (slash seen: %v), want -1", facing, ok)
	}

	e2, _ := newEngine("abc")
	e2.WalkTo(doc.Pos{Line: 0, Col: 1})
	runFor(e2, 0.5)
	e2.ShootAt(doc.Pos{Line: 0, Col: 1}) // own cell, facing right
	facing = 0
	ok = runUntil(e2, 2, func() bool {
		if v := e2.View(); v.Agent.State == agent.StateSlash {
			facing = v.Agent.Facing
			return true
		}
		return false
	})
	if !ok || facing != 1 {
		t.Fatalf("own-cell slash facing = %d (slash seen: %v), want 1", facing, ok)
	}
}

func TestCaretAndAgentSurviveRemoteOps(t *testing.T) {
	// The multiplayer host case: remote edits flood the shared doc while
	// the LOCAL keyboard still steps the cursor and the agent follows it.
	base := doc.New()
	base.Load("aa\nbb\ncc\ndd\nee\nff\ngg\nhh")
	obs := netplay.NewObservedDoc(base, nil) // wrapper without an emitter
	e := New(obs, testGrid)
	runFor(e, 0.5)

	// A remote peer deletes line 0 entirely.
	obs.ApplyRemote(netplay.OpForDelete(doc.Pos{Line: 0, Col: 0}, doc.Pos{Line: 1, Col: 0}))
	runFor(e, 0.5)

	// Local arrow: turn + step right, exactly like solo play.
	e.Step(1)
	if got := e.Caret(); got != (doc.Pos{Line: 0, Col: 1}) {
		t.Fatalf("caret = %+v after arrow, want {0 1}", got)
	}
	// The agent must glue itself to the caret (followCaret) afterwards.
	wantX, wantY := testGrid.AgentOrigin(e.Caret(), agent.SpriteH)
	ok := runUntil(e, 2, func() bool {
		v := e.View()
		return v.Agent.X == wantX && v.Agent.Y == wantY
	})
	if !ok {
		v := e.View()
		t.Fatalf("agent at (%v,%v), want (%v,%v) on the caret", v.Agent.X, v.Agent.Y, wantX, wantY)
	}
}

func TestWalkingPlaysFootsteps(t *testing.T) {
	e, _ := newEngine("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	e.WalkTo(doc.Pos{Line: 0, Col: 30})
	heard := false
	runUntil(e, 3, func() bool {
		for _, s := range e.View().Sounds {
			if s == fx.SoundStep {
				heard = true
			}
		}
		return e.View().Agent.State == agent.StateIdle // arrived
	})
	if !heard {
		t.Fatal("a walk must cue the footstep sound")
	}
}
