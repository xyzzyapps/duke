package render

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	_ "image/png" // register the PNG decoder
)

// frameNames maps a pose to its PNG file name inside a sprite folder.
// A user sheet is sprites/<name>/<frame>.png; missing files fall back to
// the built-in duke frame of the same pose, so swapping one PNG (say
// idle.png) changes just that pose.
var frameNames = map[frameID]string{
	frameIdle:       "idle",
	frameWalk0:      "walk0",
	frameWalk1:      "walk1",
	frameWalk2:      "walk2",
	frameWalk3:      "walk3",
	frameAim:        "aim",
	frameShotgun:    "shotgun",
	frameRocket:     "rocket",
	frameKatana:     "katana",
	frameKatanaDraw: "kdraw",
	frameWin:        "win",
}

// diskSheetDir is the user sprite directory (next to sounds/, relative to
// the working directory). Every subfolder becomes a character sheet in
// Settings > Sprite Sheet.
const diskSheetDir = "sprites"

// loadDiskSheets scans sprites/<name>/<frame>.png. Sheets are the folder
// names; any pose whose PNG is missing uses the built-in duke frame.
func loadDiskSheets() (map[string]*spriteSheet, error) {
	return loadDiskSheetsFrom(diskSheetDir)
}

// loadDiskSheetsFrom scans base/<name>/<frame>.png (see loadDiskSheets).
func loadDiskSheetsFrom(base string) (map[string]*spriteSheet, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, nil // no folder: built-ins only
	}
	out := map[string]*spriteSheet{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		raw := map[frameID]*image.RGBA{}
		any := false
		for id := frameID(0); id < frameCount; id++ {
			p := filepath.Join(base, name, frameNames[id]+".png")
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			img, _, err := image.Decode(bytes.NewReader(b))
			if err != nil {
				continue
			}
			raw[id] = resizeNearest(img, artW, artH)
			any = true
		}
		if !any {
			continue // empty folder: not a sheet
		}
		s := &spriteSheet{}
		for id := frameID(0); id < frameCount; id++ {
			if r := raw[id]; r != nil {
				s.raw[id] = r
				continue
			}
			f, err := parseFrame(frameArt[id]) // fall back to duke's pose
			if err != nil {
				return nil, err
			}
			s.raw[id] = f
		}
		out[name] = s
	}
	return out, nil
}

// resizeNearest resamples any image to w x h with nearest-neighbour
// filtering, so arbitrary user PNGs land on the art grid.
func resizeNearest(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	for y := 0; y < h; y++ {
		sy := y * sh / h
		for x := 0; x < w; x++ {
			sx := x * sw / w
			r, g, b, a := src.At(sx, sy).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)})
		}
	}
	return dst
}

// ExportBuiltinSheets writes every built-in character sheet as editable
// PNGs into base/<name>/<frame>.png, scaled 4x for easy pixel editing.
// The game loads them back from the sprites/ folder (see loadDiskSheets);
// cmd/gensprites regenerates them after an art change.
func ExportBuiltinSheets(base string) error {
	for sheetName, art := range allSheets {
		dir := filepath.Join(base, sheetName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for id := frameID(0); id < frameCount; id++ {
			img, err := parseFrame(art[id])
			if err != nil {
				return fmt.Errorf("%s/%s: %w", sheetName, frameNames[id], err)
			}
			big := image.NewRGBA(image.Rect(0, 0, artW*4, artH*4))
			for y := 0; y < artH; y++ {
				for x := 0; x < artW; x++ {
					c := img.RGBAAt(x, y)
					for dy := 0; dy < 4; dy++ {
						for dx := 0; dx < 4; dx++ {
							big.SetRGBA(x*4+dx, y*4+dy, c)
						}
					}
				}
			}
			f, err := os.Create(filepath.Join(dir, frameNames[id]+".png"))
			if err != nil {
				return err
			}
			err = png.Encode(f, big)
			f.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
