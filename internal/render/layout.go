// Package render draws the editor: the text grid, the gunman, projectiles,
// particles and the HUD.
//
// Coordinate spaces:
//   - world pixels: the document grid (see internal/grid); the gunman walks
//     and bullets fly here. Cell (line, col) is cellW x cellH world pixels.
//   - screen pixels: world + the scrolling camera (Layout.Scroll) + the
//     viewport origin. The window is fixed-size; everything on screen lives
//     in the same space except the HUD bars, which do not scroll.
//
// The font is hajimehoshi/bitmapfont (6x16 halfwidth cells at 1x), drawn at
// Scale=2 so a glyph exactly fills half a cell... precisely: cellW =
// advance*Scale and cellH = (ascent+descent)*Scale, so glyphs fill cells
// edge to edge and the whole screen reads as a chunky DOS terminal.
package render

import (
	"math"

	"shooter/internal/doc"
	"shooter/internal/grid"
)

// Layout describes screen geometry and the scrolling camera. It is pure
// arithmetic (no GPU resources), so it is unit-tested directly.
type Layout struct {
	grid.Grid
	ScreenW, ScreenH int     // window size in screen pixels
	OriginX, OriginY int     // text viewport top-left on screen
	ViewW, ViewH     int     // text viewport size
	ScrollX, ScrollY float64 // camera: world pixel at the viewport's top-left
}

// NewLayout builds the layout for a cols x rows text viewport with equal
// top/bottom HUD bar heights.
func NewLayout(g grid.Grid, cols, rows, barH int) Layout {
	return Layout{
		Grid:    g,
		ScreenW: cols * g.CellW,
		ScreenH: rows*g.CellH + 2*barH + TabBarHeight,
		OriginX: 0,
		OriginY: barH + TabBarHeight, // top bar + tab strip above text
		ViewW:   cols * g.CellW,
		ViewH:   rows * g.CellH,
	}
}

// WorldToScreen converts world pixels to screen pixels.
func (l Layout) WorldToScreen(x, y float64) (float64, float64) {
	return float64(l.OriginX) + x - l.ScrollX, float64(l.OriginY) + y - l.ScrollY
}

// ScreenToCell maps a screen pixel to a clamped buffer cell. ok is false
// when the point lies outside the text viewport (HUD bars, for example).
func (l Layout) ScreenToCell(sx, sy int, d doc.Document) (doc.Pos, bool) {
	if sx < l.OriginX || sx >= l.OriginX+l.ViewW ||
		sy < l.OriginY || sy >= l.OriginY+l.ViewH {
		return doc.Pos{}, false
	}
	wx := float64(sx-l.OriginX) + l.ScrollX
	wy := float64(sy-l.OriginY) + l.ScrollY
	return d.Clamp(l.Grid.PosOf(wx, wy)), true
}

// follow computes the clamped target camera for an agent centre point and
// eases the scroll toward it. The camera keeps the agent inside the middle
// band of the viewport and never scrolls past the document bounds.
func (l *Layout) follow(cx, cy float64, d doc.Document, dt float64) {
	const band = 0.3 // agent is kept within the middle 40% of the view

	// Horizontal target.
	tx := l.ScrollX
	if rel := cx - l.ScrollX; rel < float64(l.ViewW)*band {
		tx = cx - float64(l.ViewW)*band
	} else if rel > float64(l.ViewW)*(1-band) {
		tx = cx - float64(l.ViewW)*(1-band)
	}
	// Vertical target.
	ty := l.ScrollY
	if rel := cy - l.ScrollY; rel < float64(l.ViewH)*band {
		ty = cy - float64(l.ViewH)*band
	} else if rel > float64(l.ViewH)*(1-band) {
		ty = cy - float64(l.ViewH)*(1-band)
	}

	// Clamp against the document bounds (never scroll into the void).
	maxX := 0
	for i := 0; i < d.LineCount(); i++ {
		if w := d.RuneCount(i) * l.CellW; w > maxX {
			maxX = w
		}
	}
	maxX = max(0, maxX-l.ViewW)
	maxY := max(0, d.LineCount()*l.CellH-l.ViewH)
	tx = clamp(tx, 0, float64(maxX))
	ty = clamp(ty, 0, float64(maxY))

	// Ease the camera (snappy but not instantaneous).
	k := 1 - math.Pow(0.0001, dt) // ~99.99% of the way in one second
	l.ScrollX += (tx - l.ScrollX) * k
	l.ScrollY += (ty - l.ScrollY) * k
	if math.Abs(l.ScrollX-tx) < 0.05 {
		l.ScrollX = tx
	}
	if math.Abs(l.ScrollY-ty) < 0.05 {
		l.ScrollY = ty
	}
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
