// Package actions is the editor's action layer: it turns high-level
// commands (type a rune, shoot a glyph, drag a line, undo...) into animated
// sequences of agent poses, projectiles and document mutations.
//
// Core rules:
//
//   - The caret is the insertion point; the gunman always ends up standing
//     on it (the agent auto-follows the caret whenever it is idle).
//   - Mutating commands (type, shoot, drag, undo/redo) are queued FIFO and
//     only start when the world is "free": no letter in flight, no bullet
//     in flight, no line swap running, no locked agent animation
//     (aim/fire/recoil/win) and no fire cooldown. Document mutations are
//     therefore strictly serialized.
//   - Walks are NOT queued: WalkTo moves the caret and retargets the agent
//     immediately (they never mutate the document), which keeps chained
//     arrow presses and mouse clicks responsive.
//   - Projectiles capture their target cell (and the caret they were aimed
//     from) when they spawn, so a caret move mid-flight can neither retarget
//     nor corrupt them; a shot only restores the caret if it has not moved
//     since the aim began.
package actions

import (
	"log"
	"math"

	"shooter/internal/agent"
	"shooter/internal/doc"
	"shooter/internal/fx"
	"shooter/internal/grid"
)

// Timing/tuning constants (seconds unless noted).
const (
	letterFlight = 0.11 // a thrown letter's flight time
	bulletBase   = 0.07 // pistol round flight floor
	bulletSpeed  = 2200 // px/s across the buffer (distance-scaled shots)
	shotgunBase  = 0.09 // shotgun pellets are slower
	shotgunSpeed = 1400
	rocketBase   = 0.30 // the rocket is slow and dramatic
	rocketSpeed  = 900
	hitCooldown  = 0.03 // pause after a mutation is applied
	failCooldown = 0.20 // pause after a rejected command (edge of buffer)
	statusDur    = 1.4  // default lifetime of a status message
	swapDur      = 0.16 // line-drag animation duration
	maxQueue     = 256  // command queue cap
	maxShootQ    = 6    // queued shots cap (auto-fire backpressure)
)

// Engine is the command interface the UI layer talks to. Implemented by
// *engine; mocked in UI tests.
type Engine interface {
	// Tick advances the simulation by dt seconds.
	Tick(dt float64)
	// TypeRune queues one character of text input.
	TypeRune(r rune)
	// Shoot queues a caret-derived shot: back=true hits the glyph before
	// the caret, back=false the glyph at/after it. Within katana reach he
	// swings the sword; farther targets get the pistol.
	Shoot(back bool)
	// ShootAt queues a shot at an explicit glyph (right-click). He fires
	// from where he stands — the caret does not move.
	ShootAt(p doc.Pos)
	// ShootWord queues a shotgun blast that deletes the word before
	// (back=true) or after the caret (Ctrl+Backspace / Ctrl+Delete).
	ShootWord(back bool)
	// KillLine queues a rocket launcher strike that kills the line:
	// back=false to end of line or the newline itself (Emacs Ctrl+K),
	// back=true from line start to the caret (shell Ctrl+U).
	KillLine(back bool)
	// WalkTo queues a walk to a buffer position; the caret moves there.
	WalkTo(p doc.Pos)
	// MoveLine queues a one-line drag of the caret's line up (-1)/down (+1).
	MoveLine(dir int)
	// Undo/Redo queue history jumps.
	Undo()
	Redo()
	// Celebrate queues the save victory pose.
	Celebrate()
	// Cancel drops all queued commands and in-flight projectiles and
	// teleports the agent onto the caret (used when the file is replaced).
	Cancel()
	// SwapDocument retargets the engine at another buffer (tab switch):
	// pending work is dropped, the caret is restored from the tab and the
	// agent is parked there immediately.
	SwapDocument(d doc.Document, caret doc.Pos)
	// Caret returns the logical insertion point (the agent's cell).
	Caret() doc.Pos
	// View returns the render snapshot. Its FX slice drains the pending
	// effects and is only valid until the next Tick.
	View() View
	// Status returns the current transient message ("" when expired).
	Status() string
	// SetStatus overwrites the transient message (file errors etc.).
	SetStatus(msg string)
}

// View is the per-frame render snapshot of the simulation.
type View struct {
	Agent   agent.Snapshot // gunman pose
	Caret   doc.Pos        // insertion point (== the agent's cell)
	Letters []Letter       // thrown letters in flight
	Bullets []Bullet       // shots in flight
	Swaps   []Swap         // line-drag animations in progress
	Status  string         // transient HUD message ("" if none)
	FX      []fx.Effect    // effects to spawn this frame (drained)
	Sounds  []fx.Sound     // sound cues to play this frame (drained)
}

// Letter is a rune flying from the gun tip into the buffer (the target cell
// and caret are captured on the engine when it spawns).
type Letter struct {
	R            rune
	T            float64 // progress 0..1
	FromX, FromY float64
	ToX, ToY     float64
}

// Bullet is a shot in flight toward an aim point (its base caret and
// direction are captured on the engine when the aim begins). Dur is the
// flight time in seconds, chosen from the distance at spawn.
type Bullet struct {
	T            float64 // progress 0..1
	Dur          float64 // flight duration in seconds
	FromX, FromY float64
	ToX, ToY     float64
	Spread       bool // shotgun pellet fan
	Rocket       bool // rocket launcher round (drawn as a rocket)
}

// Swap animates the line drag: buffer row Row (the caret's line before the
// move) slid Dir (+1 down / -1 up) into its new row. Both the moved line
// and the displaced line are offset by the renderer while T runs 0..1.
type Swap struct {
	Row int
	Dir int
	T   float64
}

// cmdKind classifies queued commands.
type cmdKind int

const (
	cmdRune cmdKind = iota
	cmdShoot
	cmdWord
	cmdKill
	cmdMoveLine
	cmdUndo
	cmdRedo
	cmdWin
)

// cmd is one queued command.
type cmd struct {
	kind      cmdKind
	r         rune
	back      bool
	target    doc.Pos // explicit shot target (ShootAt)
	hasTarget bool
	dir       int
}

// pendingShot captures everything a shot needs when it starts; the target,
// aim point and (for word/line kills) the whole range are frozen so a
// caret jump mid-flight cannot retarget it.
type pendingShot struct {
	base      doc.Pos      // caret when the shot started (he shoots from there)
	target    doc.Pos      // glyph being attacked / FX centre
	aim       doc.Pos      // point he faces (decides facing)
	from      doc.Pos      // frozen range start (word/line kills)
	to        doc.Pos      // frozen range end
	hasRange  bool         // from/to are authoritative (word/line kills)
	fromCaret bool         // target derived from the caret (backspace/delete)
	back      bool         // caret-derived direction (only if fromCaret)
	melee     bool         // decided: katana swing instead of a gun
	weapon    agent.Weapon // Pistol / Shotgun / Rocket
}

// engine is the concrete Engine.
type engine struct {
	doc doc.Document
	ag  *agent.Agent
	g   grid.Grid

	queue       []cmd
	caret       doc.Pos
	letter      *Letter
	letterPos   doc.Pos // target cell captured when the letter was thrown
	letterCaret doc.Pos // caret captured when the letter was thrown
	bullet      *Bullet
	swap        *Swap
	shot        *pendingShot // frozen when the aim/swing starts
	cooldown    float64
	status      string
	statusT     float64
	fx          []fx.Effect
	sounds      []fx.Sound
}

// New creates an engine for the document, with the gunman starting at the
// caret (0,0).
func New(d doc.Document, g grid.Grid) Engine {
	a := agent.New(agent.DefaultConfig(), 0, 0)
	e := &engine{doc: d, ag: a, g: g}
	x, y := g.AgentOrigin(d.Clamp(doc.Pos{}), agent.SpriteH)
	a.SetPos(x, y)
	return e
}

// Caret implements Engine.
func (e *engine) Caret() doc.Pos { return e.caret }

// TypeRune implements Engine.
func (e *engine) TypeRune(r rune) {
	if len(e.queue) >= maxQueue {
		return
	}
	e.queue = append(e.queue, cmd{kind: cmdRune, r: r})
}

// Shoot implements Engine (caret-derived target).
func (e *engine) Shoot(back bool) {
	// Auto-fire backpressure: drop shots beyond the pending cap so a held
	// backspace degrades into steady automatic fire instead of a backlog.
	if n := e.countShots(); n >= maxShootQ {
		return
	}
	e.queue = append(e.queue, cmd{kind: cmdShoot, back: back})
}

// ShootAt implements Engine (explicit target, e.g. a right-clicked glyph).
func (e *engine) ShootAt(p doc.Pos) {
	if n := e.countShots(); n >= maxShootQ {
		return
	}
	e.queue = append(e.queue, cmd{kind: cmdShoot, target: e.doc.Clamp(p), hasTarget: true})
}

// countShots returns how many shoot commands are still queued.
func (e *engine) countShots() int {
	n := 0
	for _, c := range e.queue {
		if c.kind == cmdShoot || c.kind == cmdWord || c.kind == cmdKill {
			n++
		}
	}
	return n
}

// ShootWord implements Engine (shotgun on the word before/after caret).
func (e *engine) ShootWord(back bool) {
	if n := e.countShots(); n >= maxShootQ {
		return
	}
	e.queue = append(e.queue, cmd{kind: cmdWord, back: back})
}

// KillLine implements Engine (rocket launcher on the line).
func (e *engine) KillLine(back bool) {
	if n := e.countShots(); n >= maxShootQ {
		return
	}
	e.queue = append(e.queue, cmd{kind: cmdKill, back: back})
}

// WalkTo moves the caret to p and retargets the agent immediately. Walks
// never mutate the document, so they are not queued: chained arrow presses
// and mouse clicks stay responsive even while other commands are pending.
func (e *engine) WalkTo(p doc.Pos) {
	e.caret = e.doc.Clamp(p)
	x, y := e.g.AgentOrigin(e.caret, agent.SpriteH)
	e.ag.WalkTo(x, y)
}

// MoveLine implements Engine.
func (e *engine) MoveLine(dir int) {
	if dir == 0 || len(e.queue) >= maxQueue {
		return
	}
	if dir > 0 {
		dir = 1
	} else {
		dir = -1
	}
	e.queue = append(e.queue, cmd{kind: cmdMoveLine, dir: dir})
}

// Undo implements Engine.
func (e *engine) Undo() {
	if len(e.queue) < maxQueue {
		e.queue = append(e.queue, cmd{kind: cmdUndo})
	}
}

// Redo implements Engine.
func (e *engine) Redo() {
	if len(e.queue) < maxQueue {
		e.queue = append(e.queue, cmd{kind: cmdRedo})
	}
}

// Celebrate implements Engine.
func (e *engine) Celebrate() {
	e.queue = append(e.queue, cmd{kind: cmdWin})
}

// Cancel implements Engine: drops all pending work and parks the gunman at
// the start of the buffer (used when the file is replaced).
func (e *engine) Cancel() {
	log.Printf("cancel: dropping %d queued commands", len(e.queue))
	e.queue = nil
	e.letter, e.bullet, e.swap = nil, nil, nil
	e.caret = doc.Pos{}
	x, y := e.g.AgentOrigin(e.caret, agent.SpriteH)
	e.ag.SetPos(x, y)
}

// SwapDocument implements Engine (tab switch).
func (e *engine) SwapDocument(d doc.Document, caret doc.Pos) {
	log.Printf("tab swap: %d lines, caret %+v", d.LineCount(), caret)
	e.doc = d
	e.queue = nil
	e.letter, e.bullet, e.swap, e.shot = nil, nil, nil, nil
	e.fx, e.sounds = nil, nil
	e.cooldown = 0
	e.caret = d.Clamp(caret)
	x, y := e.g.AgentOrigin(e.caret, agent.SpriteH)
	e.ag.SetPos(x, y)
}

// SetStatus implements Engine.
func (e *engine) SetStatus(msg string) {
	log.Printf("status: %s", msg)
	e.status = msg
	e.statusT = statusDur
}

// Status implements Engine.
func (e *engine) Status() string {
	if e.statusT <= 0 {
		return ""
	}
	return e.status
}

// setStatus writes a transient message.
func (e *engine) setStatus(msg string, dur float64) {
	log.Printf("status: %s", msg)
	e.status = msg
	e.statusT = dur
}

// busy reports whether a command may start right now.
func (e *engine) busy() bool {
	return e.letter != nil || e.bullet != nil || e.swap != nil ||
		e.ag.Busy() || e.cooldown > 0
}

// Tick implements Engine.
func (e *engine) Tick(dt float64) {
	for _, ev := range e.ag.Tick(dt) {
		switch ev.Kind {
		case agent.EvFired:
			e.spawnBullet()
		case agent.EvSlash:
			e.applyHit() // melee connects: no projectile involved
		}
	}
	e.tickLetter(dt)
	e.tickBullet(dt)
	e.tickSwap(dt)

	if e.cooldown > 0 {
		e.cooldown -= dt
	}
	if e.statusT > 0 {
		e.statusT -= dt
		if e.statusT < 0 {
			e.statusT = 0
			e.status = ""
		}
	}
	e.pump()
}

// pump starts queued commands while the world is free and keeps the agent
// glued to the caret.
func (e *engine) pump() {
	if e.busy() {
		return
	}
	e.followCaret()
	// Start commands until one of them locks the world (or the queue is
	// drained). Several non-locking commands may start in one tick.
	for i := 0; i < 8 && !e.busy() && len(e.queue) > 0; i++ {
		c := e.queue[0]
		e.queue = e.queue[1:]
		e.start(c)
	}
}

// followCaret retargets the agent onto the caret when it is idle (or still
// walking somewhere else). Locked animations are left alone.
func (e *engine) followCaret() {
	tx, ty := e.g.AgentOrigin(e.caret, agent.SpriteH)
	x, y := e.ag.Pos()
	if x == tx && y == ty {
		return
	}
	if e.ag.State() == agent.StateIdle || e.ag.State() == agent.StateWalk {
		e.ag.WalkTo(tx, ty)
	}
}

// start executes one command's opening move.
func (e *engine) start(c cmd) {
	switch c.kind {
	case cmdRune:
		e.startLetter(c.r)
	case cmdShoot:
		e.startShoot(c)
	case cmdWord, cmdKill:
		e.startSpecial(c)
	case cmdMoveLine:
		e.startMoveLine(c.dir)
	case cmdUndo:
		e.startHistory(true)
	case cmdRedo:
		e.startHistory(false)
	case cmdWin:
		e.ag.Win()
		cx, cy := e.ag.Center()
		e.fx = append(e.fx, fx.Effect{Kind: fx.SaveFlare, X: cx, Y: cy})
		e.play(fx.SoundSave)
	}
}

// --- letters ---------------------------------------------------------------

// startLetter throws the rune from the gun tip toward the caret cell,
// capturing the target cell and caret so a caret jump mid-flight cannot
// retarget the letter.
func (e *engine) startLetter(r rune) {
	mx, my := e.ag.Muzzle()
	tx, ty := e.g.CellCenter(e.caret)
	e.letterPos = e.caret
	e.letterCaret = e.caret
	e.letter = &Letter{R: r, FromX: mx, FromY: my, ToX: tx, ToY: ty}
}

// tickLetter advances the in-flight letter; on landing it mutates the doc.
func (e *engine) tickLetter(dt float64) {
	if e.letter == nil {
		return
	}
	e.letter.T += dt / letterFlight
	if e.letter.T < 1 {
		return
	}
	// Land: insert into the captured cell. If the caret has since moved
	// (a click during the flight), the click wins and the caret stays put;
	// otherwise typing continues after the new rune.
	end := e.doc.Insert(e.letterPos, string(e.letter.R))
	if e.caret == e.letterCaret {
		e.caret = end
	} else {
		e.caret = e.doc.Clamp(e.caret)
	}
	e.fx = append(e.fx, fx.Effect{Kind: fx.Stamp, X: e.letter.ToX, Y: e.letter.ToY})
	e.play(fx.SoundStamp)
	e.letter = nil
	e.cooldown = hitCooldown
}

// --- shots -----------------------------------------------------------------
//
// Weapon rules (the Duke arsenal):
//   - Backspace/Delete/right-click: within katana reach (Chebyshev <=
//     katanaReach cells — the giant blade is long) he swings the katana;
//     farther targets get the pistol.
//   - ShootWord: always the shotgun, frozen word range.
//   - KillLine: always the rocket launcher, frozen line range.
//
// He attacks FROM his current position — the caret only moves as a
// consequence of the deletion itself.

// katanaReach is how many cells the giant blade covers (Chebyshev); the
// pistol is used beyond it.
const katanaReach = 4

// resolveTarget returns the glyph a shot aims at and whether one exists.
// Explicit targets (right-click) hit the glyph at that cell, or the line
// break at end of line; caret-derived targets follow backspace/delete
// semantics relative to base.
func (e *engine) resolveTarget(base doc.Pos, c cmd) (doc.Pos, bool) {
	if c.hasTarget {
		t := e.doc.Clamp(c.target)
		if t.Col < e.doc.RuneCount(t.Line) {
			return t, true // the glyph under the target cell
		}
		if t.Line < e.doc.LineCount()-1 {
			return t, true // the line break at end of line
		}
		return doc.Pos{}, false // nothing past the buffer's end
	}
	if c.back {
		switch {
		case base.Col > 0:
			return doc.Pos{Line: base.Line, Col: base.Col - 1}, true
		case base.Line > 0:
			prev := base.Line - 1
			return doc.Pos{Line: prev, Col: e.doc.RuneCount(prev)}, true
		}
		return doc.Pos{}, false
	}
	switch {
	case base.Col < e.doc.RuneCount(base.Line):
		return base, true // glyph under the caret (at his feet)
	case base.Line < e.doc.LineCount()-1:
		return doc.Pos{Line: base.Line + 1, Col: 0}, true // joins next line
	}
	return doc.Pos{}, false
}

// shotRange returns the range an attack deletes when it connects, derived
// from the frozen pending shot so a caret jump cannot retarget it.
func shotRange(d doc.Document, s *pendingShot) (from, to, target doc.Pos, ok bool) {
	if s.hasRange {
		// Word/line kills froze their range when the shot started.
		return s.from, s.to, s.target, true
	}
	target = s.target
	switch {
	case s.fromCaret && s.back:
		// Backspace: the glyph before the caret up to the caret itself.
		return target, s.base, target, true
	case s.fromCaret && s.base.Col < d.RuneCount(s.base.Line):
		// Forward on a line: one glyph at the caret.
		return s.base, doc.Pos{Line: s.base.Line, Col: s.base.Col + 1}, target, true
	case s.fromCaret:
		// Forward at end of line: join with the next line.
		return s.base, doc.Pos{Line: s.base.Line + 1, Col: 0}, target, true
	case target.Col < d.RuneCount(target.Line):
		// Explicit target: one glyph at that cell.
		return target, doc.Pos{Line: target.Line, Col: target.Col + 1}, target, true
	default:
		// Explicit target at end of line: join with the next line.
		return target, doc.Pos{Line: target.Line + 1, Col: 0}, target, true
	}
}

// reach returns the Chebyshev distance between two cells.
func reach(a, b doc.Pos) int {
	dl := a.Line - b.Line
	if dl < 0 {
		dl = -dl
	}
	dc := a.Col - b.Col
	if dc < 0 {
		dc = -dc
	}
	if dl > dc {
		return dl
	}
	return dc
}

// startShoot freezes the shot and picks the weapon: the katana within
// blade reach, the pistol beyond. The swing connects on EvSlash, the
// bullet spawns on EvFired.
func (e *engine) startShoot(c cmd) {
	base := e.caret
	target, ok := e.resolveTarget(base, c)
	if !ok {
		e.setStatus("NOTHING TO SHOOT", 1.0)
		e.cooldown = failCooldown
		return
	}
	melee := reach(target, base) <= katanaReach
	e.shot = &pendingShot{
		base:      base,
		target:    target,
		aim:       target,
		fromCaret: !c.hasTarget,
		back:      c.back,
		melee:     melee,
		weapon:    agent.Pistol,
	}
	if melee {
		log.Printf("katana: %+v -> %+v", base, target)
		e.play(fx.SoundSlash) // the whoosh plays as the swing starts
		e.ag.Slash(facingFor(base, target))
		return
	}
	log.Printf("pistol: %+v -> %+v", base, target)
	e.ag.AimWith(facingFor(base, target), agent.Pistol)
}

// startSpecial handles the shotgun (word) and rocket (line) commands:
// the range is frozen up front and the heavy weapon is always used.
func (e *engine) startSpecial(c cmd) {
	base := e.caret
	var (
		from, to doc.Pos
		ok       bool
		weapon   agent.Weapon
		label    string
	)
	if c.kind == cmdWord {
		from, to, ok = wordRange(e.doc, base, c.back)
		weapon, label = agent.Shotgun, "shotgun"
	} else {
		from, to, ok = killRange(e.doc, base, c.back)
		weapon, label = agent.Rocket, "rocket"
	}
	if !ok {
		e.setStatus("NOTHING TO KILL", 1.0)
		e.cooldown = failCooldown
		return
	}
	aim := from
	if !c.back {
		aim = to
	}
	e.shot = &pendingShot{
		base:     base,
		aim:      aim,
		target:   midpoint(from, to),
		from:     from,
		to:       to,
		hasRange: true,
		weapon:   weapon,
	}
	log.Printf("%s: %+v -> %+v", label, base, aim)
	e.ag.AimWith(facingFor(base, aim), weapon)
}

// facingFor turns him toward an aim point relative to the shot's base;
// vertical aims default to facing left.
func facingFor(base, aim doc.Pos) int {
	if aim.Col > base.Col {
		return 1
	}
	return -1
}

// midpoint returns a representative cell inside [from, to) for FX.
func midpoint(from, to doc.Pos) doc.Pos {
	if from.Line != to.Line {
		return from
	}
	return doc.Pos{Line: from.Line, Col: (from.Col + to.Col) / 2}
}

// wordRange returns the range a word-kill deletes, staying on the line:
// back skips blank cells then the word before the caret; forward skips
// the word then its trailing blanks (shell/Emacs kill-word style).
func wordRange(d doc.Document, base doc.Pos, back bool) (from, to doc.Pos, ok bool) {
	runes := []rune(d.Line(base.Line))
	i := base.Col
	if i > len(runes) {
		i = len(runes)
	}
	if back {
		start := i
		for start > 0 && isBlank(runes[start-1]) {
			start--
		}
		for start > 0 && !isBlank(runes[start-1]) {
			start--
		}
		if start == i {
			return doc.Pos{}, doc.Pos{}, false
		}
		return doc.Pos{Line: base.Line, Col: start}, base, true
	}
	end := i
	for end < len(runes) && !isBlank(runes[end]) {
		end++
	}
	for end < len(runes) && isBlank(runes[end]) {
		end++
	}
	if end == i {
		return doc.Pos{}, doc.Pos{}, false
	}
	return base, doc.Pos{Line: base.Line, Col: end}, true
}

// killRange returns the range a line-kill deletes:
// back=false (Emacs Ctrl+K) kills to end of line, or the newline itself
// when the caret sits at end of line (joining with the next line);
// back=true (shell Ctrl+U) kills from line start to the caret.
func killRange(d doc.Document, base doc.Pos, back bool) (from, to doc.Pos, ok bool) {
	n := d.RuneCount(base.Line)
	if back {
		if base.Col == 0 {
			return doc.Pos{}, doc.Pos{}, false
		}
		return doc.Pos{Line: base.Line}, base, true
	}
	if base.Col < n {
		return base, doc.Pos{Line: base.Line, Col: n}, true
	}
	if base.Line < d.LineCount()-1 {
		return base, doc.Pos{Line: base.Line + 1, Col: 0}, true
	}
	return doc.Pos{}, doc.Pos{}, false
}

// isBlank reports whether a rune counts as word padding.
func isBlank(r rune) bool { return r == ' ' || r == '\t' }

// spawnBullet launches the projectile when the gun goes off. Flight time
// and FX follow the weapon; the pistol's scales with distance.
func (e *engine) spawnBullet() {
	if e.shot == nil || e.shot.melee {
		return // defensive: no projectile for a katana swing
	}
	mx, my := e.ag.Muzzle()
	tx, ty := e.g.CellCenter(e.shot.target)
	dist := math.Hypot(tx-mx, ty-my)
	b := &Bullet{FromX: mx, FromY: my, ToX: tx, ToY: ty}
	switch e.shot.weapon {
	case agent.Shotgun:
		b.Dur = shotgunBase + dist/shotgunSpeed
		b.Spread = true
		e.play(fx.SoundShotgun)
		e.fx = append(e.fx, fx.Effect{Kind: fx.Muzzle, X: mx, Y: my})
		e.fx = append(e.fx, fx.Effect{Kind: fx.Blast, X: mx, Y: my})
	case agent.Rocket:
		b.Dur = rocketBase + dist/rocketSpeed
		b.Rocket = true
		e.play(fx.SoundRocket)
		e.fx = append(e.fx, fx.Effect{Kind: fx.Blast, X: mx, Y: my})
	default:
		b.Dur = bulletBase + dist/bulletSpeed
		e.play(fx.SoundPistol)
		e.fx = append(e.fx, fx.Effect{Kind: fx.Muzzle, X: mx, Y: my})
	}
	e.bullet = b
}

// tickBullet advances the shot; on impact the hit is applied.
func (e *engine) tickBullet(dt float64) {
	if e.bullet == nil {
		return
	}
	dur := e.bullet.Dur
	if dur <= 0 {
		dur = 0.1
	}
	e.bullet.T += dt / dur
	if e.bullet.T < 1 {
		return
	}
	e.bullet = nil
	e.applyHit()
}

// applyHit performs the frozen shot's deletion (punch connect or bullet
// impact) and reports it as shatter FX.
func (e *engine) applyHit() {
	if e.shot == nil {
		return
	}
	s := e.shot
	e.shot = nil
	from, to, target, ok := shotRange(e.doc, s)
	if !ok {
		return
	}
	deleted := e.doc.Delete(from, to)
	e.caret = e.doc.Clamp(shiftCaret(e.caret, from, to))
	tx, ty := e.g.CellCenter(target)
	if s.weapon == agent.Rocket {
		e.fx = append(e.fx, fx.Effect{Kind: fx.Explosion, X: tx, Y: ty})
		e.play(fx.SoundShotgun) // the impact boom
	}
	e.fx = append(e.fx, fx.Effect{Kind: fx.Shatter, X: tx, Y: ty, Runes: deleted})
	e.cooldown = hitCooldown
}

// shiftCaret moves the caret out of a deleted range [from, to): text to
// its left disappearing pulls it back, deleted lines collapse into the
// join point, and lines below a join shift up.
func shiftCaret(c, from, to doc.Pos) doc.Pos {
	if to.Line == from.Line {
		switch {
		case c.Line == from.Line && c.Col > to.Col:
			c.Col -= to.Col - from.Col
		case c.Line == from.Line && c.Col > from.Col:
			c.Col = from.Col
		}
		return c
	}
	switch {
	case c.Line == from.Line && c.Col > from.Col:
		c.Col = from.Col
	case c.Line == to.Line:
		return doc.Pos{Line: from.Line, Col: from.Col + (c.Col - to.Col)}
	case c.Line > to.Line:
		c.Line -= to.Line - from.Line
	}
	return c
}

// --- line drags ------------------------------------------------------------

// startMoveLine swaps the caret's line with its neighbour and animates it.
func (e *engine) startMoveLine(dir int) {
	row := e.caret.Line
	if !e.doc.MoveLine(row, dir) {
		e.setStatus("EDGE OF THE BUFFER", 1.0)
		e.cooldown = failCooldown
		return
	}
	// The document already swapped; animate both rows back to their old
	// positions and let them slide into place.
	e.swap = &Swap{Row: row, Dir: dir}
	e.caret = doc.Pos{Line: row + dir, Col: e.caret.Col}
	x, y := e.g.AgentOrigin(e.caret, agent.SpriteH)
	e.ag.DragTo(x, y, swapDur)
}

// tickSwap advances the line-drag animation.
func (e *engine) tickSwap(dt float64) {
	if e.swap == nil {
		return
	}
	e.swap.T += dt / swapDur
	if e.swap.T >= 1 {
		e.swap = nil
	}
}

// --- history ---------------------------------------------------------------

// startHistory undoes/redoes and moves the caret to the affected spot.
func (e *engine) startHistory(undo bool) {
	var h doc.Hint
	var ok bool
	if undo {
		h, ok = e.doc.Undo()
	} else {
		h, ok = e.doc.Redo()
	}
	if !ok {
		if undo {
			e.setStatus("NOTHING TO UNDO", 1.0)
		} else {
			e.setStatus("NOTHING TO REDO", 1.0)
		}
		e.cooldown = failCooldown
		return
	}
	col := e.caret.Col
	if !h.KeepCol {
		col = h.Pos.Col
	}
	e.caret = e.doc.Clamp(doc.Pos{Line: h.Pos.Line, Col: col})
	cx, cy := e.g.CellCenter(e.caret)
	e.fx = append(e.fx, fx.Effect{Kind: fx.Puff, X: cx, Y: cy})
}

// --- view ------------------------------------------------------------------

// play queues a sound cue for the next View (drained once per frame).
func (e *engine) play(s fx.Sound) {
	e.sounds = append(e.sounds, s)
}

// View implements Engine.
func (e *engine) View() View {
	v := View{
		Agent:  e.ag.Snapshot(),
		Caret:  e.caret,
		Status: e.Status(),
		FX:     e.fx,
		Sounds: e.sounds,
	}
	if e.letter != nil {
		v.Letters = append(v.Letters, *e.letter)
	}
	if e.bullet != nil {
		v.Bullets = append(v.Bullets, *e.bullet)
	}
	if e.swap != nil {
		v.Swaps = append(v.Swaps, *e.swap)
	}
	e.fx = nil
	e.sounds = nil
	return v
}
