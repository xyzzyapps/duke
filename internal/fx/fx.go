// Package fx carries small fire-and-forget effect descriptions.
//
// The simulation (internal/actions) appends effects when something worth
// drawing happens; the renderer drains them each frame and turns them into
// short-lived particles/flashes, and the shell plays the sound cues. Both
// are pure data: no rendering or audio imports here, so the action layer
// stays testable without a GPU or a sound device.
package fx

// Kind classifies a visual effect.
type Kind int

const (
	// Shatter is a glyph blown apart at (X, Y); Runes is the destroyed
	// text whose fragments should spray outward.
	Shatter Kind = iota
	// Muzzle is the flash at the gun tip when a shot is fired.
	Muzzle
	// Blast is the wide shotgun muzzle flare.
	Blast
	// Explosion is the rocket's impact fireball.
	Explosion
	// SaveFlare celebrates a successful save.
	SaveFlare
	// Puff marks an undo/redo jump.
	Puff
)

// Effect is one description handed to the renderer.
type Effect struct {
	Kind  Kind
	X, Y  float64 // world pixels (impact/origin point)
	Runes string  // destroyed text (Shatter only)
}

// Sound identifies a sound cue. The shell maps these to the synthesised
// samples in internal/audio; values must stay aligned with that package.
type Sound int

const (
	// SoundSlash: the katana whoosh (played when the swing starts).
	SoundSlash Sound = iota
	// SoundPistol: the pistol crack.
	SoundPistol
	// SoundShotgun: the deep boom.
	SoundShotgun
	// SoundRocket: launch whoosh.
	SoundRocket
	// SoundStamp: a typed letter lands in the buffer.
	SoundStamp
	// SoundSave: the save chime.
	SoundSave
)

// String makes cue names readable in logs.
func (s Sound) String() string {
	switch s {
	case SoundSlash:
		return "slash"
	case SoundPistol:
		return "pistol"
	case SoundShotgun:
		return "shotgun"
	case SoundRocket:
		return "rocket"
	case SoundStamp:
		return "stamp"
	case SoundSave:
		return "save"
	}
	return "unknown"
}
