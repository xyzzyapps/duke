package agent

import "testing"

const dt = 1.0 / 60.0

// tickFor runs the agent for n ticks, returning the last event batch.
func tickFor(a *Agent, n int) []Event {
	var evs []Event
	for i := 0; i < n; i++ {
		evs = a.Tick(dt)
	}
	return evs
}

func TestWalkArrivesAndEmitsOnce(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	a.WalkTo(32, 0)
	if a.State() != StateWalk {
		t.Fatalf("state = %v, want walk", a.State())
	}
	arrived := 0
	// 32px at 160px/s = 0.2s = ~12 ticks; run well past that.
	for i := 0; i < 60; i++ {
		for _, e := range a.Tick(dt) {
			if e.Kind == EvArrived {
				arrived++
			}
		}
	}
	if arrived != 1 {
		t.Fatalf("arrived = %d, want 1", arrived)
	}
	if a.State() != StateIdle {
		t.Fatalf("state = %v, want idle", a.State())
	}
	x, y := a.Pos()
	if x != 32 || y != 0 {
		t.Fatalf("pos = (%v,%v), want (32,0)", x, y)
	}
}

func TestWalkArrivesImmediatelyWhenAtTarget(t *testing.T) {
	a := New(DefaultConfig(), 5, 5)
	a.WalkTo(5, 5)
	evs := a.Tick(dt)
	if len(evs) != 1 || evs[0].Kind != EvArrived {
		t.Fatalf("evs = %v, want single arrival", evs)
	}
	if a.State() != StateIdle {
		t.Fatalf("state = %v, want idle", a.State())
	}
}

func TestWalkIsAxisLockedHorizontalFirst(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	a.WalkTo(32, 32)
	tickFor(a, 1)
	// After one tick only the horizontal leg may have moved.
	_, y := a.Pos()
	if y != 0 {
		t.Fatalf("y = %v during horizontal leg, want 0", y)
	}
	// Long enough to finish both legs.
	for i := 0; i < 120 && a.State() == StateWalk; i++ {
		a.Tick(dt)
	}
	x, y := a.Pos()
	if x != 32 || y != 32 {
		t.Fatalf("pos = (%v,%v), want (32,32)", x, y)
	}
}

func TestAimFireRecoilSequence(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	a.Aim(-1)
	if a.Facing() != -1 {
		t.Fatalf("facing = %d, want -1", a.Facing())
	}
	fired := 0
	for i := 0; i < 60; i++ {
		for _, e := range a.Tick(dt) {
			if e.Kind == EvFired {
				fired++
			}
		}
	}
	if fired != 1 {
		t.Fatalf("fired = %d, want 1", fired)
	}
	if a.State() != StateIdle {
		t.Fatalf("state = %v, want idle after sequence", a.State())
	}
}

func TestWalkCancelsAim(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	a.Aim(1)
	a.WalkTo(16, 0)
	if a.State() != StateWalk {
		t.Fatalf("state = %v, want walk", a.State())
	}
	// No shot must ever happen.
	for i := 0; i < 60; i++ {
		for _, e := range a.Tick(dt) {
			if e.Kind == EvFired {
				t.Fatal("walk must cancel the pending shot")
			}
		}
	}
}

func TestMuzzleMirrorsWithFacing(t *testing.T) {
	a := New(DefaultConfig(), 10, 10)
	// Orientation swap: facing right fires from the sprite''s left edge,
	// facing left from its right edge.
	rx, ry := a.Muzzle()
	if rx <= 10+SpriteW/2 {
		t.Fatalf("right-facing muzzle x = %v, want right half", rx)
	}
	a.Aim(-1)
	lx, ly := a.Muzzle()
	if lx >= 10+SpriteW/2 {
		t.Fatalf("left-facing muzzle x = %v, want left half", lx)
	}
	if ry != ly {
		t.Fatalf("muzzle y changed with facing: %v vs %v", ry, ly)
	}
}

func TestDragIsTimedAndEased(t *testing.T) {
	cfg := DefaultConfig()
	a := New(cfg, 0, 0)
	a.DragTo(0, 32, 0.3)
	if a.State() != StateDrag {
		t.Fatalf("state = %v, want drag", a.State())
	}
	// Halfway through the drag it must not have arrived yet but should be
	// strictly between start and end.
	n := int(0.15 / dt)
	tickFor(a, n)
	_, y := a.Pos()
	if y <= 0 || y >= 32 {
		t.Fatalf("y mid-drag = %v, want strictly between 0 and 32", y)
	}
	// Run past the end of the drag and count the arrival event.
	arrived := 0
	for i := 0; i < 60; i++ {
		for _, e := range a.Tick(dt) {
			if e.Kind == EvArrived {
				arrived++
			}
		}
	}
	_, y = a.Pos()
	if y != 32 || arrived == 0 {
		t.Fatalf("y = %v arrived = %d, want 32 and at least 1", y, arrived)
	}
}

func TestWinReturnsToIdle(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	a.Win()
	if a.State() != StateWin {
		t.Fatalf("state = %v, want win", a.State())
	}
	tickFor(a, 120)
	if a.State() != StateIdle {
		t.Fatalf("state = %v, want idle", a.State())
	}
}

func TestSetPosTeleportsAndResets(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	a.Win()
	a.SetPos(64, 32)
	x, y := a.Pos()
	if x != 64 || y != 32 || a.State() != StateIdle {
		t.Fatalf("pos = (%v,%v) state = %v", x, y, a.State())
	}
}

func TestBusyFlagsLockedAnimations(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	if a.Busy() {
		t.Fatal("idle agent must not be busy")
	}
	a.Aim(1)
	if !a.Busy() {
		t.Fatal("aiming agent must be busy")
	}
	a.WalkTo(16, 0)
	if a.Busy() {
		t.Fatal("walking agent must not be busy (walks do not block commands)")
	}
}

func TestSnapshotExposesState(t *testing.T) {
	a := New(DefaultConfig(), 7, 9)
	a.Aim(1)
	s := a.Snapshot()
	if s.X != 7 || s.Y != 9 || s.State != StateAim || s.Facing != 1 {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestSlashSequence(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	a.Slash(-1)
	if a.Facing() != -1 {
		t.Fatalf("facing = %d, want -1", a.Facing())
	}
	if a.Busy() != true {
		t.Fatal("punching must count as busy (locks the command queue)")
	}
	punched, fired := 0, 0
	for i := 0; i < 60; i++ {
		for _, e := range a.Tick(dt) {
			switch e.Kind {
			case EvSlash:
				punched++
			case EvFired:
				fired++
			}
		}
	}
	if punched != 1 {
		t.Fatalf("punched = %d, want exactly 1", punched)
	}
	if fired != 0 {
		t.Fatal("a slash must never fire the gun")
	}
	if a.State() != StateIdle {
		t.Fatalf("state = %v, want idle after the swing", a.State())
	}
}

func TestWalkCancelsSlash(t *testing.T) {
	a := New(DefaultConfig(), 0, 0)
	a.Slash(1)
	a.WalkTo(16, 0)
	for i := 0; i < 60; i++ {
		for _, e := range a.Tick(dt) {
			if e.Kind == EvSlash {
				t.Fatal("walk must cancel the pending slash")
			}
		}
	}
}

func TestWalkFollowsDirection(t *testing.T) {
	a := New(DefaultConfig(), 64, 0)
	// Walking left: he faces left while travelling.
	a.WalkTo(0, 0)
	sawLeft := false
	for i := 0; i < 240 && a.State() == StateWalk; i++ {
		a.Tick(dt)
		if a.Facing() == -1 {
			sawLeft = true
		}
	}
	if !sawLeft {
		t.Fatal("walking left must face left")
	}
	// Walking right: he flips back and stays right.
	a.WalkTo(64, 0)
	sawRight := false
	for i := 0; i < 240 && a.State() == StateWalk; i++ {
		a.Tick(dt)
		if a.Facing() != 1 {
			t.Fatalf("facing = %d while walking right, want 1", a.Facing())
		}
		sawRight = true
	}
	if !sawRight {
		t.Fatal("walk right never ran")
	}
}

func TestWalkEmitsFootstepsOnContactBeats(t *testing.T) {
	a := New(Config{}, 0, 0)
	a.SetPos(0, 0)
	a.WalkTo(0, 4*48) // walk down four rows
	steps := 0
	for i := 0; i < 200; i++ {
		for _, ev := range a.Tick(1.0 / 60) {
			if ev.Kind == EvStep {
				steps++
			}
		}
	}
	// The gait advances 9 phase-frames/s over a 4-frame cycle: two contact
	// beats per cycle (phase crossing 2 and wrapping through 0) => 4.5
	// footfalls per second while walking, then silence once he arrives.
	if steps < 4 || steps > 22 {
		t.Fatalf("footfalls = %d over 200 ticks, want 4..22 (4.5/s while walking)", steps)
	}
}
