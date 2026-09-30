// Package grid maps buffer cells (line, col) to world pixels and back.
//
// The action layer works in world pixels (where the gunman walks and
// shoots) while the document works in cells; the renderer adds a scrolling
// camera on top. Keeping the mapping here lets internal/actions and
// internal/render share it without an import cycle.
//
// World pixel space: cell (line, col) occupies the rectangle
// [col*CellW, (line+1)*CellW) x [line*CellH, (line+1)*CellH).
package grid

import (
	"math"

	"shooter/internal/doc"
)

// Grid carries the character cell size in pixels.
type Grid struct {
	CellW int
	CellH int
}

// CellOrigin returns the top-left corner of the cell in world pixels.
func (g Grid) CellOrigin(p doc.Pos) (float64, float64) {
	return float64(p.Col * g.CellW), float64(p.Line * g.CellH)
}

// CellCenter returns the centre of the cell in world pixels (the visual
// centre of a glyph; used as the aim point for shots and letters).
func (g Grid) CellCenter(p doc.Pos) (float64, float64) {
	x, y := g.CellOrigin(p)
	return x + float64(g.CellW)/2, y + float64(g.CellH)/2
}

// AgentOrigin returns the sprite top-left corner for an agent occupying p,
// so that its feet rest on the bottom edge of the row (the sprite is
// shorter than a row).
func (g Grid) AgentOrigin(p doc.Pos, spriteH float64) (float64, float64) {
	x, _ := g.CellOrigin(p)
	return x, float64((p.Line+1)*g.CellH) - spriteH
}

// PosOf returns the cell containing the world pixel (x, y). Coordinates
// outside the grid map to negative or oversized indices; callers clamp
// against the document.
func (g Grid) PosOf(x, y float64) doc.Pos {
	return doc.Pos{
		Line: int(math.Floor(y / float64(g.CellH))),
		Col:  int(math.Floor(x / float64(g.CellW))),
	}
}
