// Package agent implements the gunman: the character that replaces the text
// cursor. He walks the buffer, types letters that stamp into place, aims and fires at
// glyphs, drags lines and celebrates saves.
//
// The agent is a pure animation/movement state machine that knows nothing
// about the text buffer. It reports discrete edge events (arrived, fired)
// from Tick; the action layer (internal/actions) decides what those mean for
// the document. Positions are world pixels: cell (line, col) maps to world
// pixels through the renderer's layout, and the sprite's top-left corner is
// the agent position (feet rest on the bottom of the row it occupies).
package agent

// State is the gunman's animation state.
type State int

const (
	StateIdle   State = iota // standing, breathing
	StateWalk                // walking to a target
	StateAim                 // gun raised, about to shoot
	StateFire                // muzzle flash frame
	StateRecoil              // kick-back frame
	StateSlash               // melee: giant katana swing at a nearby glyph
	StateDrag                // timed slide (used for line moves)
	StateWin                 // celebration pose
)

// String implements fmt.Stringer for readable logs/tests.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateWalk:
		return "walk"
	case StateAim:
		return "aim"
	case StateFire:
		return "fire"
	case StateRecoil:
		return "recoil"
	case StateSlash:
		return "slash"
	case StateDrag:
		return "drag"
	case StateWin:
		return "win"
	}
	return "unknown"
}

// Weapon selects which armed pose (and projectile) the gunman uses while
// aiming. The katana has no Weapon value: melee is the Slash state.
type Weapon int

const (
	Pistol Weapon = iota
	Shotgun
	Rocket
)

// EventKind classifies edge events emitted by Tick.
type EventKind int

const (
	// EvArrived: a walk/drag finished; the agent is idle at its target.
	EvArrived EventKind = iota
	// EvFired: the gun went off this tick; spawn a projectile at Muzzle().
	EvFired
	// EvSlash: the melee strike connected this tick; apply the hit
	// directly (no projectile).
	EvSlash
	// EvStep: a footfall on the walk cycle's contact beats (the shell
	// plays the step sound).
	EvStep
)

// Event is one edge notification from Tick.
type Event struct {
	Kind EventKind
}

// Sprite dimensions in pixels. The art is 12x16 and drawn at the font's
// Scale (2), so the sprite is 24x32 on screen: exactly one layout row tall
// (feet rest on the row's bottom edge, the head never clips under the HUD)
// and two cells wide. The gunman covers his own cell plus the cell to his
// right; the glyph he is about to shoot (to his left) always stays visible.
const (
	// World-pixel size of the sprite: the 12x16 art sheet rendered at the
	// global art scale (render.Scale = 3), i.e. exactly two cells wide and
	// one cell tall.
	SpriteW = 36.0
	SpriteH = 48.0
)

// Config holds movement/animation tuning (seconds and pixels).
type Config struct {
	WalkSpeed float64 // px/s while walking
	AimDur    float64 // gun raised before the shot
	FireDur   float64 // muzzle flash duration
	RecoilDur float64 // kick-back duration
	SlashHit  float64 // the katana connects (EvSlash)
	SlashDur  float64 // total swing duration
	WinDur    float64 // celebration duration
}

// DefaultConfig returns the tuned defaults.
func DefaultConfig() Config {
	return Config{
		WalkSpeed: 160,
		AimDur:    0.10,
		FireDur:   0.05,
		RecoilDur: 0.09,
		SlashHit:  0.16,
		SlashDur:  0.30,
		WinDur:    0.7,
	}
}

// Snapshot is a read-only view of the agent for the renderer.
type Snapshot struct {
	X, Y   float64 // sprite top-left in world pixels
	Facing int     // +1 right, -1 left
	State  State
	Gun    Weapon  // armed pose while aiming/firing
	Frame  int     // frame index within the current state's animation
	T      float64 // seconds spent in the current state
}

// Agent is the gunman entity. Not safe for concurrent use.
type Agent struct {
	cfg Config

	x, y    float64 // sprite top-left, world pixels
	facing  int
	gun     Weapon
	state   State
	t       float64 // seconds in the current state
	phase   float64 // walk cycle accumulator
	slashed bool    // melee connect already emitted for this strike

	// movement
	fromX, fromY float64
	toX, toY     float64
	moveT        float64 // seconds elapsed for timed moves
	moveDur      float64 // >0 for timed (drag) moves, 0 for speed-based

	evs []Event // reused buffer returned by Tick
}

// New creates an agent at the given sprite position.
func New(cfg Config, x, y float64) *Agent {
	return &Agent{cfg: cfg, x: x, y: y, facing: 1}
}

// Pos returns the sprite top-left corner in world pixels.
func (a *Agent) Pos() (float64, float64) { return a.x, a.y }

// SetPos teleports the agent and resets it to idle (used on file load).
func (a *Agent) SetPos(x, y float64) {
	a.x, a.y = x, y
	a.facing = 1
	a.state = StateIdle
	a.t = 0
	a.moveDur = 0
}

// State returns the current animation state.
func (a *Agent) State() State { return a.state }

// Facing returns +1 (right) or -1 (left).
func (a *Agent) Facing() int { return a.facing }

// Busy reports whether the agent is busy with a locked animation
// (aiming/firing/slashing/celebrating); walks do not count as busy.
func (a *Agent) Busy() bool {
	switch a.state {
	case StateAim, StateFire, StateRecoil, StateSlash, StateWin:
		return true
	}
	return false
}

// WalkTo starts a speed-based walk toward (x, y), cancelling any aim.
// Orientation follows the direction of travel: walking left turns him
// left, walking right turns him right.
func (a *Agent) WalkTo(x, y float64) {
	a.fromX, a.fromY = a.x, a.y
	a.toX, a.toY = x, y
	a.moveDur = 0
	a.state = StateWalk
	a.t = 0
}

// DragTo slides the agent to (x, y) over exactly dur seconds (used to keep
// the gunman glued to a line he is dragging).
func (a *Agent) DragTo(x, y float64, dur float64) {
	a.fromX, a.fromY = a.x, a.y
	a.toX, a.toY = x, y
	a.moveDur = dur
	a.moveT = 0
	a.state = StateDrag
	a.t = 0
}

// Aim raises the pistol in the given facing (+1/-1) and runs the
// aim -> fire -> recoil sequence. A pending walk is cancelled.
func (a *Agent) Aim(facing int) {
	a.AimWith(facing, Pistol)
}

// AimWith is Aim with an explicit weapon: the pose and projectile follow
// the choice (pistol, shotgun, rocket launcher).
func (a *Agent) AimWith(facing int, w Weapon) {
	if facing > 0 {
		a.facing = 1
	} else if facing < 0 {
		a.facing = -1
	}
	a.gun = w
	a.state = StateAim
	a.t = 0
}

// Slash throws a melee strike in the given facing (+1/-1). The hit
// connects (EvSlash) partway through the swing; no projectile is
// involved. A pending walk is cancelled.
func (a *Agent) Slash(facing int) {
	if facing > 0 {
		a.facing = 1
	} else if facing < 0 {
		a.facing = -1
	}
	a.state = StateSlash
	a.t = 0
	a.slashed = false
}

// Face turns the gunman in place to look toward dir (+1 right, -1 left)
// without moving. Used by the turn-first input model: a directional press
// in the opposite direction only turns him; the next press acts.
func (a *Agent) Face(dir int) {
	if dir < 0 {
		a.facing = -1
	} else {
		a.facing = 1
	}
}

// Win plays the celebration pose (after a save).
func (a *Agent) Win() {
	a.state = StateWin
	a.t = 0
}

// Muzzle returns the gun tip in world pixels, honouring facing. The art's
// gun sits at mid height, which is exactly SpriteH/2 after the 2x draw.
func (a *Agent) Muzzle() (float64, float64) {
	if a.facing < 0 {
		return a.x + 3, a.y + SpriteH/2
	}
	return a.x + SpriteW - 3, a.y + SpriteH/2
}

// Center returns the sprite centre in world pixels.
func (a *Agent) Center() (float64, float64) {
	return a.x + SpriteW/2, a.y + SpriteH/2
}

// Tick advances the state machine by dt seconds and returns edge events.
// The returned slice is only valid until the next call to Tick.
func (a *Agent) Tick(dt float64) []Event {
	a.evs = a.evs[:0]
	a.t += dt

	switch a.state {
	case StateWalk:
		a.walk(dt)
	case StateDrag:
		a.drag(dt)
	case StateAim:
		if a.t >= a.cfg.AimDur {
			a.state = StateFire
			a.t = 0
			a.evs = append(a.evs, Event{Kind: EvFired})
		}
	case StateSlash:
		if !a.slashed && a.t >= a.cfg.SlashHit {
			a.slashed = true
			a.evs = append(a.evs, Event{Kind: EvSlash})
		}
		if a.t >= a.cfg.SlashDur {
			a.state = StateIdle
			a.t = 0
			a.slashed = false
		}
	case StateFire:
		if a.t >= a.cfg.FireDur {
			a.state = StateRecoil
			a.t = 0
		}
	case StateRecoil:
		if a.t >= a.cfg.RecoilDur {
			a.state = StateIdle
			a.t = 0
		}
	case StateWin:
		if a.t >= a.cfg.WinDur {
			a.state = StateIdle
			a.t = 0
		}
	case StateIdle:
		// breathing only
	}
	// Advance the leg-cycle animation while moving. Footfalls land on the
	// contact beats (phase crossing 2, and wrapping back through 0).
	if a.state == StateWalk || a.state == StateDrag {
		prev := a.phase
		a.phase += dt * 9 // 9 frames per second walk cycle
		if a.phase >= walkFrames {
			a.phase -= walkFrames
		}
		if (prev < 2 && a.phase >= 2) || prev > a.phase {
			a.evs = append(a.evs, Event{Kind: EvStep})
		}
	} else {
		a.phase = 0
	}
	return a.evs
}

// walk advances a speed-based walk; arrival emits EvArrived.
// Movement is axis-locked (horizontal leg first, then vertical) so the
// agent reads as moving along the text grid rather than drifting diagonally.
func (a *Agent) walk(dt float64) {
	dx, dy := a.toX-a.x, a.toY-a.y
	if abs(dx) < 0.5 && abs(dy) < 0.5 {
		a.x, a.y = a.toX, a.toY
		a.state = StateIdle
		a.t = 0
		a.evs = append(a.evs, Event{Kind: EvArrived})
		return
	}
	step := a.cfg.WalkSpeed * dt
	// Orientation follows horizontal travel: left walk faces left,
	// right walk faces right.
	if abs(dx) > 0.5 {
		if dx > 0 {
			a.facing = 1
		} else {
			a.facing = -1
		}
	}
	if abs(dx) > 0.5 {
		move := min(step, abs(dx))
		if dx > 0 {
			a.x += move
		} else {
			a.x -= move
		}
		return
	}
	move := min(step, abs(dy))
	if dy > 0 {
		a.y += move
	} else {
		a.y -= move
	}
}

// drag advances a timed slide; arrival emits EvArrived.
func (a *Agent) drag(dt float64) {
	if a.moveDur <= 0 {
		a.x, a.y = a.toX, a.toY
		a.state = StateIdle
		a.t = 0
		a.evs = append(a.evs, Event{Kind: EvArrived})
		return
	}
	a.moveT += dt
	if a.moveT >= a.moveDur {
		a.x, a.y = a.toX, a.toY
		a.state = StateIdle
		a.t = 0
		a.evs = append(a.evs, Event{Kind: EvArrived})
		return
	}
	k := a.moveT / a.moveDur
	// Ease-in-out so the dragged line starts and stops softly.
	e := k * k * (3 - 2*k)
	a.x = a.fromX + (a.toX-a.fromX)*e
	a.y = a.fromY + (a.toY-a.fromY)*e
}

// walkFrames is the number of frames in the walk cycle.
const walkFrames = 4

// Snapshot returns the renderer view of the agent.
func (a *Agent) Snapshot() Snapshot {
	s := Snapshot{X: a.x, Y: a.y, Facing: a.facing, State: a.state, Gun: a.gun, T: a.t}
	switch a.state {
	case StateWalk, StateDrag:
		s.Frame = int(a.phase) % walkFrames
	case StateIdle:
		s.Frame = int(a.t*3) % 2 // slow 2-frame breathing
	}
	return s
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
