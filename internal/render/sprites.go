package render

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"shooter/internal/agent"
)

// artW x artH is the 18x24 art grid every user sprite sheet lands on
// (any PNG is resized to it) and the agent's logical sprite size at
// Scale (36x48). Raw art faces LEFT; facing right is derived by
// mirroring at draw time.
const (
	artW = 18
	artH = 24
)

// frameID indexes the sprite sheet. Each pose is one PNG inside a sheet
// folder (sprites/<name>/<frame>.png; see sprite_disk.go — there are no
// built-in frames, every sheet comes from disk).
type frameID int

const (
	frameIdle frameID = iota
	frameWalk0
	frameWalk1
	frameWalk2
	frameWalk3
	frameAim
	frameShotgun
	frameRocket
	frameKatana
	frameKatanaDraw
	frameWin
	frameCount
)

// spriteSheet holds the parsed frames and their lazily created GPU images
// (Ebitengine images are only built once the first draw happens).
type spriteSheet struct {
	raw   [frameCount]*image.RGBA
	gpu   [frameCount]*ebiten.Image
	built bool
}

// image returns the GPU image for a frame, uploading it on first use.
func (s *spriteSheet) image(id frameID) *ebiten.Image {
	if !s.built {
		for i, raw := range s.raw {
			if raw != nil {
				s.gpu[i] = ebiten.NewImageFromImage(raw)
			}
		}
		s.built = true
	}
	return s.gpu[id]
}

// pose picks the frame plus per-state draw offsets for a snapshot.
// Offsets are in art pixels and are applied before mirroring, so they flip
// with the facing automatically. Aiming uses the frame for the weapon in
// his hands; melee uses the katana stance, except right at the start of a
// swing when he is still reaching back to the sheath.
func (s *spriteSheet) pose(a agent.Snapshot) (id frameID, xOff, yOff int) {
	switch a.State {
	case agent.StateIdle:
		// Slow breathing bob: the empty art row above his head absorbs the
		// rise, so his feet stay planted on the row line at all times.
		return frameIdle, 0, -(int(a.T*3) % 2)
	case agent.StateWalk:
		f := a.Frame % 4
		y := 0
		if f == 1 || f == 3 {
			y = -1 // passing pose: body rides up a pixel
		}
		return frameID(frameWalk0 + frameID(f)), 0, y
	case agent.StateAim, agent.StateFire:
		switch a.Gun {
		case agent.Shotgun:
			return frameShotgun, 0, 0
		case agent.Rocket:
			return frameRocket, 0, 0
		}
		return frameAim, 0, 0
	case agent.StateRecoil:
		x := 1 // kick back (the base art faces left, so +x = his back)
		switch a.Gun {
		case agent.Shotgun:
			return frameShotgun, x, 0
		case agent.Rocket:
			return frameRocket, x, 0
		}
		return frameAim, x, 0
	case agent.StateSlash:
		if a.T < 0.12 {
			return frameKatanaDraw, 0, 0 // one hand still at the sheath
		}
		return frameKatana, 0, 0 // both hands on the hilt, cutting
	case agent.StateDrag:
		return frameKatana, 0, 0 // hands on the line
	case agent.StateWin:
		return frameWin, 0, -2 // little hop
	}
	return frameIdle, 0, 0
}
