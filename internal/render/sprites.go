package render

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"shooter/internal/agent"
)

// The gunman is Duke Nukem: platinum blond flat-top, thick black
// wraparound glasses, red tank top, gold Nuke belt buckle, blue jeans and
// black steel-tipped boots (the Duke Nukem 3D look).
//
// The art is artW x artH (12x16) and drawn at the font's Scale (2), so the
// sprite occupies 24x32 screen pixels: exactly one layout row tall (feet on
// the row's bottom edge, head never clipped by the HUD) and two cells wide.
// Art pixels therefore have the same size as font pixels.
//
// Seven of the sixteen rows are the head (big head, thick glasses). The
// frames cover his arsenal: idle with the pistol at his hip, a four-pose
// walk cycle, pistol aim, shotgun, rocket launcher, the katana stance
// (the giant blade itself is drawn by the renderer as a swinging vector)
// and the victory pose.
const (
	artW = 12
	artH = 16
)

// frameID indexes the sprite sheet.
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
	frameWin
	frameCount
)

// palette maps art characters to colours.
var palette = map[byte]color.RGBA{
	'.': {0, 0, 0, 0},         // transparent
	'k': {10, 12, 16, 255},    // outline
	'h': {245, 220, 95, 255},  // platinum blond flat-top
	'g': {26, 30, 44, 255},    // sunglasses lens
	'G': {150, 195, 245, 255}, // lens glint (shine)
	's': {236, 178, 134, 255}, // skin
	'S': {194, 138, 98, 255},  // skin/jaw shade
	'r': {206, 52, 52, 255},   // red tank top
	'R': {146, 34, 40, 255},   // red tank shade
	'b': {58, 80, 170, 255},   // blue jeans
	'n': {236, 186, 56, 255},  // gold Nuke belt buckle
	'p': {30, 30, 38, 255},    // black boots
	't': {156, 162, 172, 255}, // steel toe tips
	'w': {214, 218, 228, 255}, // gun (light)
	'd': {110, 114, 126, 255}, // gun (dark)
	'o': {146, 92, 46, 255},   // shotgun wood forend
}

// Shared art fragments keep every frame consistent.
var (
	headRows = []string{
		".khhhhhhhhk.", // wide flat-top
		".khhhhhhhhk.",
		"..khhhhhhk..", // hair narrowing into the face
		"..kgGsggsk..", // glasses: left lens (glint) + right lens
		"..kggsggsk..", // glasses: lower row of both 2x2 lenses
		"..kssssssk..", // face
		"..kssSSssk..", // jaw with mouth shadow
	}
	// torsoRows: four rows — red tank with bare arms, pistol hanging at
	// the hip, gold buckle belt, jeans hips.
	torsoRows = []string{
		"..ksrrrrsk..",
		"..ksrrrrskd.", // pistol grip beside the hip
		"..kbbnnbbkw.", // belt + buckle + pistol barrel
		"..kbbbbbbk..",
	}
	baseLegs = []string{
		"..kbbk.kbbk.",
		"..kbbk.kbbk.",
		"..kptk.kptk.", // steel toes
		"..kkkk.kkkk.",
	}
	// belt/hip rows shared by the armed poses (no hip gun while aiming).
	lowerRows = []string{
		"..kbbnnbbk..",
		"..kbbbbbbk..",
	}
)

// full assembles empty row + head + torso + legs into a complete frame.
func full(torso, legs []string) []string {
	rows := make([]string, 0, artH)
	rows = append(rows, "............")
	rows = append(rows, headRows...)
	rows = append(rows, torso...)
	rows = append(rows, legs...)
	return rows
}

// frameArt holds the ASCII art for every frame (artW columns x artH rows).
var frameArt = map[frameID][]string{
	frameIdle:  full(torsoRows, baseLegs),
	frameWalk0: full(torsoRows, baseLegs),
	frameWalk1: full(torsoRows, []string{
		"..kbbkkbbk..",
		"..kbbkkbbk..",
		"..kptkkptk..",
		"..kkkkkkkk..",
	}),
	frameWalk2: full(torsoRows, []string{
		".kbbk..kbbk.",
		".kbbk..kbbk.",
		".kptk..kptk.",
		".kkkk..kkkk.",
	}),
	frameWalk3: full(torsoRows, []string{
		"..kbbkkbbk..",
		"..kbbkkbbk..",
		"..kptkkptk..",
		"..kkkkkkkk..",
	}),
	// Pistol: arm out, barrel, grip.
	frameAim: full(append([]string{
		"..ksrrrrswww",
		"..ksrrrrsd..",
	}, lowerRows...), baseLegs),
	// Shotgun: double-height barrel over the wood forend.
	frameShotgun: full(append([]string{
		"..ksrrrrswww",
		"..ksrrrrsoo.",
	}, lowerRows...), baseLegs),
	// Rocket launcher: thick tube with a red warhead at the tip.
	frameRocket: full(append([]string{
		"..ksrrrrswdr",
		"..ksrrrrsd..",
	}, lowerRows...), baseLegs),
	// Katana stance: both arms thrust forward (the giant blade is drawn
	// by the renderer as a swinging vector, not part of the art).
	frameKatana: full(append([]string{
		"..ksrrrrssss",
		"..ksrrrrssk.",
	}, lowerRows...), baseLegs),
	// Victory: arms raised beside his big head, hands above the flat-top.
	frameWin: func() []string {
		rows := make([]string, 0, artH)
		rows = append(rows, ".s........s.") // hands
		rows = append(rows, ".shhhhhhhhs.") // arms + hair (wide)
		rows = append(rows, ".shhhhhhhhs.")
		rows = append(rows, ".skhhhhhhks.") // arms + hair (narrow)
		rows = append(rows, ".skgGsggsk..") // arms + glasses
		rows = append(rows, ".skggsggsk..")
		rows = append(rows, ".skssssssks.") // arms + face
		rows = append(rows, ".skssSSssks.") // arms + jaw
		rows = append(rows,
			"..krrrrrrk..", // chest (arms are up, no side arms)
			"..krrrrrrk..",
			"..kbbnnbbk..",
			"..kbbbbbbk..",
		)
		rows = append(rows, baseLegs...)
		return rows
	}(),
}

// parseFrame converts ASCII art into an RGBA image. It is pure CPU work
// (no GPU), so it can be unit-tested without a window.
func parseFrame(rows []string) (*image.RGBA, error) {
	if len(rows) != artH {
		return nil, fmt.Errorf("frame has %d rows, want %d", len(rows), artH)
	}
	img := image.NewRGBA(image.Rect(0, 0, artW, artH))
	for y, row := range rows {
		if len(row) != artW {
			return nil, fmt.Errorf("row %d has %d columns, want %d", y, len(row), artW)
		}
		for x := 0; x < artW; x++ {
			c, ok := palette[row[x]]
			if !ok {
				return nil, fmt.Errorf("row %d col %d: unknown palette char %q", y, x, row[x])
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img, nil
}

// spriteSheet holds the parsed frames and their lazily created GPU images
// (Ebitengine images are only built once the first draw happens).
type spriteSheet struct {
	raw   [frameCount]*image.RGBA
	gpu   [frameCount]*ebiten.Image
	built bool
}

// newSpriteSheet parses every frame's art.
func newSpriteSheet() (*spriteSheet, error) {
	s := &spriteSheet{}
	for id, rows := range frameArt {
		img, err := parseFrame(rows)
		if err != nil {
			return nil, fmt.Errorf("frame %d: %w", id, err)
		}
		s.raw[id] = img
	}
	// Every frameID must have art (the map could miss one).
	for id := frameID(0); id < frameCount; id++ {
		if s.raw[id] == nil {
			return nil, fmt.Errorf("frame %d has no art", id)
		}
	}
	return s, nil
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
// his hands; melee and line drags use the katana stance.
func (s *spriteSheet) pose(a agent.Snapshot) (id frameID, xOff, yOff int) {
	switch a.State {
	case agent.StateIdle:
		// Slow breathing bob: the empty art row above his head absorbs the
		// rise, so his feet stay planted on the row line at all times.
		return frameIdle, 0, -(int(a.T*3) % 2)
	case agent.StateWalk:
		return frameID(frameWalk0 + frameID(a.Frame%4)), 0, 0
	case agent.StateAim, agent.StateFire:
		switch a.Gun {
		case agent.Shotgun:
			return frameShotgun, 0, 0
		case agent.Rocket:
			return frameRocket, 0, 0
		}
		return frameAim, 0, 0
	case agent.StateRecoil:
		x := -1 // kick back
		switch a.Gun {
		case agent.Shotgun:
			return frameShotgun, x, 0
		case agent.Rocket:
			return frameRocket, x, 0
		}
		return frameAim, x, 0
	case agent.StateSlash, agent.StateDrag:
		return frameKatana, 0, 0 // hands on the hilt / on the line
	case agent.StateWin:
		return frameWin, 0, -2 // little hop
	}
	return frameIdle, 0, 0
}
