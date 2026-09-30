package grid

import (
	"testing"

	"shooter/internal/doc"
)

func TestCellOriginAndCenter(t *testing.T) {
	g := Grid{CellW: 16, CellH: 32}
	x, y := g.CellOrigin(doc.Pos{Line: 2, Col: 3})
	if x != 48 || y != 64 {
		t.Fatalf("origin = (%v,%v), want (48,64)", x, y)
	}
	cx, cy := g.CellCenter(doc.Pos{Line: 2, Col: 3})
	if cx != 56 || cy != 80 {
		t.Fatalf("center = (%v,%v), want (56,80)", cx, cy)
	}
}

func TestAgentOriginStandsOnRowBottom(t *testing.T) {
	g := Grid{CellW: 16, CellH: 32}
	x, y := g.AgentOrigin(doc.Pos{Line: 1, Col: 2}, 32)
	if x != 32 {
		t.Fatalf("x = %v, want 32", x)
	}
	// Feet (y + 32) must rest on the bottom of row 1 (2*32 = 64).
	if y+32 != 64 {
		t.Fatalf("feet at %v, want 64", y+32)
	}
}

func TestPosOfRoundTrip(t *testing.T) {
	g := Grid{CellW: 16, CellH: 32}
	p := doc.Pos{Line: 5, Col: 9}
	x, y := g.CellCenter(p)
	if got := g.PosOf(x, y); got != p {
		t.Fatalf("PosOf = %+v, want %+v", got, p)
	}
}

func TestPosOfHandlesNegativePixels(t *testing.T) {
	g := Grid{CellW: 16, CellH: 32}
	if got := g.PosOf(-1, -1); got != (doc.Pos{Line: -1, Col: -1}) {
		t.Fatalf("PosOf(-1,-1) = %+v", got)
	}
}
