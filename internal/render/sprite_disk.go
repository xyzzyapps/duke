package render

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"

	_ "image/png" // register the PNG decoder
)

// frameNames maps a pose to its PNG file name inside a sprite folder.
// A sheet is sprites/<name>/<frame>.png; a missing PNG becomes a neutral
// placeholder box so an incomplete sheet stays visible (the old built-in
// duke art fallback is gone - every sheet comes from disk now).
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
// names; any pose whose PNG is missing gets a placeholder box.
func loadDiskSheets() (map[string]*spriteSheet, error) {
	return loadDiskSheetsFrom(diskSheetDir)
}

// loadDiskSheetsFrom scans base/<name>/<frame>.png (see loadDiskSheets).
func loadDiskSheetsFrom(base string) (map[string]*spriteSheet, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, nil // no folder: no sheets (the game requires sprites/)
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
			s.raw[id] = placeholderFrame() // missing pose: visible box
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

// placeholderFrame is drawn for a pose whose PNG is missing from a sheet:
// a plain bordered box, clearly "art missing" without pretending to be a
// character.
func placeholderFrame() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, artW, artH))
	for y := 0; y < artH; y++ {
		for x := 0; x < artW; x++ {
			c := color.RGBA{30, 36, 46, 255}
			if x == 0 || y == 0 || x == artW-1 || y == artH-1 {
				c = color.RGBA{201, 209, 217, 110}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}
