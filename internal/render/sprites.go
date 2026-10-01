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
// black steel-tipped boots (the Duke Nukem 3D look). He carries a
// **machine gun** held at the ready in front of him and the **katana
// sheathed across his back**. The pixel array is the LEFT-facing base:
// facing right is derived by mirroring at draw time.
//
// The art is artW x artH (18x24) and drawn at the font scale (2), so the
// sprite occupies 36x48 screen pixels: exactly one layout row tall (feet
// on the row's bottom edge, head never clipped by the HUD) and two cells
// wide. Every art pixel is 2x2 screen pixels.
//
// Row plan (24 rows): 1 empty (breathing margin), 10 head rows, 8 torso
// rows, 5 leg rows. The frames cover his arsenal: idle with the machine
// gun out front, a four-pose walk cycle, machine-gun aim, shotgun, rocket
// launcher, the katana stance, the katana draw (reaching to the sheath)
// and the victory pose. The giant katana blade itself is drawn by the
// renderer as a swinging vector.
const (
	artW = 18
	artH = 24
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
	frameKatanaDraw
	frameWin
	frameCount
)

// palette maps art characters to colours.
var palette = map[byte]color.RGBA{
	'.': {0, 0, 0, 0},         // transparent
	'k': {10, 12, 16, 255},    // outline
	'h': {245, 220, 95, 255},  // platinum blond flat-top
	'H': {255, 240, 150, 255}, // sun-bleached flat-top crown
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

// Shared art fragments keep every frame consistent. The gun faces LEFT
// (he holds it out front); the katana sheath rides his back on the RIGHT.
var (
	headRows = []string{
		".HHHHHHHHHHHHHHHH.", // sun-bleached flat-top crown
		".HHHHHHHHHHHHHHHH.", // crown (bright)
		".khhhhhhhhhhhhhhk.", // flat-top, square corners
		".ksgGGgggggGGgsk..", // wraparound shades: glare per lens
		".ksgggggggggggsk..", // shades: solid dark band (shaved sides beside)
		"..kSssssssssSsk...", // cheeks with shading
		"..kssssskkkssssk..", // mouth: short firm line
		"..ksssssssssssk...", // chin
		"..ksssssssssssk...", // jaw
		".kssssssssssssk...", // jaw (wider)
	}
	// torsoRows: 8 rows — shoulders, the machine gun cradled out front
	// (barrel, receiver, magazine), belt/buckle, jeans hips and the
	// katana hilt + scabbard on the back edge.
	torsoRows = []string{
		"..ksrrrrrrrrrrsk..", // shoulders + arms
		"wwwwwwssrrrrrskn..", // SMG barrel + both hands + katana guard
		"..wwwkssrrrrrskdd.", // receiver + hands + scabbard
		"..kssrrrrrrrrsSk..", // chest below the receiver
		"..kbbbbnnnnbbbbk..", // belt + gold buckle
		"..kddddbbbbbbbbk..", // magazine under the belt
		"..kbbbbbbbbbbbbk..", // hips
		"..kbbbbk..kbbbbk..", // legs split (crotch)
	}
	lowerRows = []string{
		"..kbbbbnnnnbbbbk..", // belt + buckle
		"..kbbbbbbbbbbbbk..", // hips
	}
	baseLegs = []string{
		".kbbbbbk..kbbbbbk.", // thighs
		".kbbbbbk..kbbbbbk.", // knees
		".kpppppk..kpppppk.", // boot shafts
		".tttpppk...tttpppk", // boots + steel toes (facing left)
		".kkkkkkk...kkkkkkk", // sole
	}
)

// full assembles empty row + head + torso + legs into a complete frame.
func full(torso, legs []string) []string {
	rows := make([]string, 0, artH)
	rows = append(rows, "..................")
	rows = append(rows, headRows...)
	rows = append(rows, torso...)
	rows = append(rows, legs...)
	return rows
}

// frameArt holds the ASCII art for every frame (artW columns x artH rows).
var frameArt = map[frameID][]string{
	frameIdle: full(torsoRows, baseLegs),
	frameWalk0: full(torsoRows, []string{
		// contact A: right foot planted, left foot lifted mid-swing
		".kbbbbbk..kbbbbbk.",
		".kbbbbbk..kbbbbbk.",
		".kpppppk..kbbbkk..",
		".ttpppk...kkkkk...",
		".kkkkkkk..........",
	}),
	frameWalk1: full(torsoRows, []string{
		// passing: legs together, body rides up a pixel (pose yOff)
		".kbbbbbk..kbbbbbk.",
		".kbbbbbk..kbbbbbk.",
		".kpppppk..kpppppk.",
		".tttpppk...tttpppk",
		".kkkkkkk...kkkkkkk",
	}),
	frameWalk2: full(torsoRows, []string{
		// contact B: left foot planted, right foot lifted mid-swing
		".kbbbbbk..kbbbbbk.",
		".kbbbbbk..kbbbbbk.",
		".kbbbkk..kpppppk..",
		".kkkkkk...ttpppk..",
		"...........kkkkkkk",
	}),
	frameWalk3: full(torsoRows, []string{
		// passing: legs together again
		".kbbbbbk..kbbbbbk.",
		".kbbbbbk..kbbbbbk.",
		".kpppppk..kpppppk.",
		".tttpppk...tttpppk",
		".kkkkkkk...kkkkkkk",
	}),
	frameAim: full([]string{
		"..ksrrrrrrrrrrsk..",
		"wwwwwwwssrrrrskn..", // barrel raised, longer, both hands
		"..wwwdkssrrrrskdd.",
		"..kssrrrrrrrrsSk..",
		"..kbbbbnnnnbbbbk..",
		"..kddddbbbbbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbk..kbbbbk..",
	}, baseLegs),
	frameShotgun: full([]string{
		"..ksrrrrrrrrrrsk..",
		"wwwwwwwwssrrrskn..", // twin barrels, wider
		"ooooooooosrrrrskd.", // wood forend
		"..kssrrrrrrrrsSk..",
		"..kbbbbnnnnbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbk..kbbbbk..",
	}, baseLegs),
	frameRocket: full([]string{
		"..ksrrrrrrrrrrsk..",
		"rrrrwwwwwwwrrrskn.", // warhead + tube across the chest
		"..rrwwwwwwwrrrSkd.",
		"..kssrrrrrrrrsSk..",
		"..kbbbbnnnnbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbk..kbbbbk..",
	}, baseLegs),
	frameKatana: full([]string{
		"..ksrrrrrrrrrrsk..",
		"sssssssssrrrrrskn.", // both hands on the hilt, out front
		"..ksssrrrrrrrskd..",
		"..kssrrrrrrrrsSk..",
		"..kbbbbnnnnbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbk..kbbbbk..",
	}, baseLegs),
	frameKatanaDraw: full([]string{
		"..ksrrrrrrrrrrsk..",
		"..ksrrrrrsssssskn.", // reaching back to the sheath
		"..ksrrrrrksssskdd.",
		"..kssrrrrrrrrsSk..",
		"..kbbbbnnnnbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbbbbbbbbbk..",
		"..kbbbbk..kbbbbk..",
	}, baseLegs),
	frameWin: func() []string {
		rows := make([]string, 0, artH)
		rows = append(rows, "..................")
		rows = append(rows, ".s..............s.") // hands
		rows = append(rows, ".sHHHHHHHHHHHHHHs.")
		rows = append(rows, ".shhhhhhhhhhhhhhs.")
		rows = append(rows, ".sksggGGGGGGGGgks.")
		rows = append(rows, "..kssgggggggggsk..")
		rows = append(rows, "..kSssssssssSsk...")
		rows = append(rows, "..kssssskkkssssk..")
		rows = append(rows, "..ksssssssssssk...")
		rows = append(rows, "..ksssssssssssk...")
		rows = append(rows, ".kssssssssssssk...")
		rows = append(rows,
			"..krrrrrrrrrrrrk..", // arms up, chest bare
			"..krrrrrrrrrrrrk..",
			"..krrrrrrrrrrrrk..",
			"..kSrrrrrrrrrSrk..",
			"..kbbbbnnnnbbbbk..",
			"..kbbbbbbbbbbbbk..",
			"..kbbbbbbbbbbbbk..",
			"..kbbbbk..kbbbbk..",
			".kbbbbbk..kbbbbbk.",
			".kbbbbbk..kbbbbbk.",
			".kpppppk..kpppppk.",
			".tttpppk...tttpppk",
			".kkkkkkk...kkkkkkk",
		)
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

// allSheets maps character names to their frame art. The renderer builds
// one sprite sheet per entry; the Settings dialog lets the player pick one.
var allSheets = map[string]map[frameID][]string{
	defaultSheet: frameArt,
}

// newSpriteSheet parses every frame's art.
func newSpriteSheet(art map[frameID][]string) (*spriteSheet, error) {
	s := &spriteSheet{}
	for id, rows := range art {
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
