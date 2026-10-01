package render

import (
	"shooter/internal/actions"
	"testing"

	"shooter/internal/doc"
	"shooter/internal/grid"
)

// testLayout mirrors the runtime geometry (18x48 cells, 80x16 viewport,
// 48px HUD bars at the shared GUI face size, 44px tab strip).
func testLayout() Layout {
	return NewLayout(grid.Grid{CellW: 18, CellH: 48}, 80, 16, 48)
}

func TestNewLayoutGeometry(t *testing.T) {
	l := testLayout()
	if l.ScreenW != 1440 {
		t.Fatalf("ScreenW = %d, want 1440", l.ScreenW)
	}
	if l.ScreenH != 908 { // 16*48 + 2*48 bars + 44 tab strip
		t.Fatalf("ScreenH = %d, want 908", l.ScreenH)
	}
	if l.OriginY != 92 || l.ViewW != 1440 || l.ViewH != 768 {
		t.Fatalf("origin/view = %d %d %d (want origin 92)", l.OriginY, l.ViewW, l.ViewH)
	}
}

func TestWorldToScreenUsesScroll(t *testing.T) {
	l := testLayout()
	l.ScrollX, l.ScrollY = 100, 50
	x, y := l.WorldToScreen(130, 90)
	if x != 30 || y != float64(l.OriginY+40) {
		t.Fatalf("world(130,90) -> screen(%v,%v)", x, y)
	}
}

func TestScreenToCellRoundTrip(t *testing.T) {
	l := testLayout()
	d := doc.New()
	d.Load("hello\nworld")
	// A screen point inside the viewport maps back to the same cell.
	cellX := l.OriginX + 2*18 + 5 // col 2, inside the glyph
	cellY := l.OriginY + 1*48 + 5 // line 1
	p, ok := l.ScreenToCell(cellX, cellY, d)
	if !ok {
		t.Fatal("point should be inside the viewport")
	}
	if p != (doc.Pos{Line: 1, Col: 2}) {
		t.Fatalf("cell = %+v, want {1 2}", p)
	}
}

func TestScreenToCellOutsideViewport(t *testing.T) {
	l := testLayout()
	d := doc.New()
	if _, ok := l.ScreenToCell(10, 5, d); ok { // top HUD bar
		t.Fatal("point on the HUD bar must be rejected")
	}
	if _, ok := l.ScreenToCell(10, l.ScreenH-5, d); ok { // bottom bar
		t.Fatal("point on the HUD bar must be rejected")
	}
	if _, ok := l.ScreenToCell(l.ScreenW+1, 100, d); ok {
		t.Fatal("point past the right edge must be rejected")
	}
}

func TestScreenToCellClampsToDocument(t *testing.T) {
	l := testLayout()
	d := doc.New()
	d.Load("ab")
	// Click well beyond the buffer's end: must clamp into it.
	p, ok := l.ScreenToCell(l.ScreenW-1, l.OriginY+l.ViewH-1, d)
	if !ok {
		t.Fatal("inside viewport")
	}
	if p.Line != 0 || p.Col != 2 {
		t.Fatalf("cell = %+v, want {0 2} (clamped)", p)
	}
}

func TestScreenToCellHonoursScroll(t *testing.T) {
	l := testLayout()
	d := doc.New()
	d.Load("aaa\nbbb\nccc")
	l.ScrollY = 48 // one row down: screen shows line 1 at the top
	p, ok := l.ScreenToCell(0, l.OriginY, d)
	if !ok {
		t.Fatal("inside viewport")
	}
	if p.Line != 1 {
		t.Fatalf("line = %d, want 1 (camera scrolled)", p.Line)
	}
}

func TestFollowKeepsAgentInMiddleBand(t *testing.T) {
	l := testLayout()
	d := doc.New()
	// A tall document so scrolling is possible.
	d.Load("line\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline\nline")
	// Agent near the bottom: camera must move down but clamp to bounds.
	l.follow(60, 19*48+16, d, 1.0)
	if l.ScrollY <= 0 {
		t.Fatalf("ScrollY = %v, should have followed down", l.ScrollY)
	}
	// Camera must never exceed the document bounds.
	maxY := float64(d.LineCount()*l.CellH - l.ViewH)
	if l.ScrollY > maxY+0.01 {
		t.Fatalf("ScrollY = %v beyond max %v", l.ScrollY, maxY)
	}
	// Small document: camera stays at zero.
	small := doc.New()
	small.Load("tiny")
	l2 := testLayout()
	l2.follow(5, 10, small, 1.0)
	if l2.ScrollY != 0 || l2.ScrollX != 0 {
		t.Fatalf("scroll for tiny doc = (%v,%v), want 0,0", l2.ScrollX, l2.ScrollY)
	}
}

func TestFollowEasesTowardTarget(t *testing.T) {
	l := testLayout()
	d := doc.New()
	for i := 0; i < 60; i++ {
		d.Insert(doc.Pos{Line: i, Col: 0}, "row\n")
	}
	l.follow(60, 50*48+16, d, 1.0/60.0)
	first := l.ScrollY
	if first <= 0 {
		t.Fatalf("ScrollY = %v after one tick, want > 0", first)
	}
	l.follow(60, 50*48+16, d, 1.0/60.0)
	if l.ScrollY < first {
		t.Fatalf("ScrollY went backwards: %v -> %v", first, l.ScrollY)
	}
}

func TestSwapOffsetAnimatesBothRows(t *testing.T) {
	swaps := []actions.Swap{{Row: 3, Dir: 1, T: 0}}
	if dy := swapOffset(3, swaps, 32); dy != 32 {
		t.Fatalf("displaced row offset = %v, want 32", dy)
	}
	if dy := swapOffset(4, swaps, 32); dy != -32 {
		t.Fatalf("dragged row offset = %v, want -32", dy)
	}
	swaps[0].T = 1
	if dy := swapOffset(3, swaps, 32); dy != 0 {
		t.Fatalf("finished swap offset = %v, want 0", dy)
	}
	if dy := swapOffset(9, swaps, 32); dy != 0 {
		t.Fatalf("unrelated row offset = %v, want 0", dy)
	}
}

func TestTabBarRectsAndHit(t *testing.T) {
	titles := []string{"notes.txt", "untitled-2"}
	rects := TabBarRects(titles)
	if len(rects) != 2 {
		t.Fatalf("rects = %d", len(rects))
	}
	if rects[0].X != 8 || rects[1].X <= rects[0].X+rects[0].W {
		t.Fatalf("tabs overlap or misplaced: %+v", rects)
	}
	for _, r := range rects {
		if r.Y <= BarH || r.Y+r.H >= BarH+TabBarHeight {
			t.Fatalf("tab not inside strip: %+v", r)
		}
	}
	if i, ok := TabHit(titles, rects[1].X+5, rects[1].Y+3); !ok || i != 1 {
		t.Fatalf("hit = %d ok=%v, want 1", i, ok)
	}
	if _, ok := TabHit(titles, 10, 500); ok {
		t.Fatal("click far below the strip must miss")
	}
}

func TestTabTitleTruncates(t *testing.T) {
	if got := TabTitle(""); got != "untitled" {
		t.Fatalf("TabTitle() = %q", got)
	}
	if got := TabTitle("C:/x/notes.txt"); got != "notes.txt" {
		t.Fatalf("TabTitle(path) = %q", got)
	}
	if got := TabTitle("a-very-long-filename-above-18.txt"); len(got) > 18 {
		t.Fatalf("TabTitle long = %q (%d)", got, len(got))
	}
}
