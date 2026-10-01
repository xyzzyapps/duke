package render

import (
	"strings"
	"testing"

	"shooter/internal/actions"
	"shooter/internal/doc"
	"shooter/internal/grid"
)

// testLayout mirrors the runtime geometry (18x48 cells, the 80x20 grid,
// 48px HUD bars at the shared GUI face size, 44px tab strip).
func testLayout() Layout {
	return NewLayout(grid.Grid{CellW: 18, CellH: 48}, 80, 20, 48)
}

func TestNewLayoutGeometry(t *testing.T) {
	l := testLayout()
	if l.ScreenW != 1440 {
		t.Fatalf("ScreenW = %d, want 1440", l.ScreenW)
	}
	if l.ScreenH != 1100 { // 20*48 + 2*48 bars + 44 tab strip
		t.Fatalf("ScreenH = %d, want 1100", l.ScreenH)
	}
	if l.OriginY != 92 || l.ViewW != 1440 || l.ViewH != 960 {
		t.Fatalf("origin/view = %d %d %d (want origin 92, view 1440x960)", l.OriginY, l.ViewW, l.ViewH)
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
	// A tall document so scrolling is possible (30 rows > the 20-row view).
	d.Load(strings.Repeat("line\n", 30))
	// Agent near the bottom: camera must move down but clamp to bounds.
	l.follow(60, 29*48+16, d, 1.0)
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

func TestLayoutRowsAndColsAccessors(t *testing.T) {
	l := testLayout()
	if l.Rows() != 20 || l.Cols() != 80 {
		t.Fatalf("Rows/Cols = %d/%d, want 20/80", l.Rows(), l.Cols())
	}
}

func TestScrollbarLockFreezesTheCamera(t *testing.T) {
	l := testLayout()
	d := doc.New()
	d.Load(strings.Repeat("row\n", 40)) // 40*48 = 1920 > 960 viewport
	l.follow(60, 39*48+16, d, 1.0)
	if l.ScrollY <= 0 {
		t.Fatal("camera should have followed the caret")
	}
	// Lock: the caret may move anywhere, the vertical camera stays put.
	l.LockScrollY()
	before := l.ScrollY
	l.follow(60, 0, d, 1.0)
	l.follow(60, 0, d, 1.0)
	if l.ScrollY != before {
		t.Fatalf("locked camera moved: %v -> %v", before, l.ScrollY)
	}
	// The thumb jump sets a fraction of the document height (0 = top,
	// 1 = bottom).
	l.SetScrollYFrac(0, d)
	if l.ScrollY != 0 {
		t.Fatalf("top frac = %v, want 0", l.ScrollY)
	}
	maxY := float64(d.LineCount()*l.CellH - l.ViewH)
	l.SetScrollYFrac(1, d)
	if l.ScrollY < maxY-1 || l.ScrollY > maxY+1 {
		t.Fatalf("bottom frac = %v, want ~%v", l.ScrollY, maxY)
	}
	// Unlock: the caret-follow resumes.
	l.UnlockScrollY()
	l.follow(60, 0, d, 1.0)
	l.follow(60, 0, d, 1.0)
	if l.ScrollY != 0 {
		t.Fatalf("camera did not resume after unlock: %v", l.ScrollY)
	}
}

func TestScrollbarZonesOnlyWhenOverflowing(t *testing.T) {
	l := testLayout()
	small := doc.New()
	small.Load("hi")
	if v, h := l.ScrollbarZones(l.OriginX+l.ViewW-1, l.OriginY+24, small); v || h {
		t.Fatalf("short doc exposes scrollbar zones: v=%v h=%v", v, h)
	}
	tall := doc.New()
	tall.Load(strings.Repeat("row\n", 40))
	v, _ := l.ScrollbarZones(l.OriginX+l.ViewW-1, l.OriginY+24, tall)
	if !v {
		t.Fatal("tall doc must expose the vertical zone")
	}
	if _, h := l.ScrollbarZones(l.OriginX+l.ViewW-1, l.OriginY+24, tall); h {
		t.Fatal("vertical zone only: column overflow absent")
	}
	wide := doc.New()
	wide.Load("x" + strings.Repeat("y", 200))
	_, h := l.ScrollbarZones(l.OriginX+30, l.OriginY+l.ViewH-1, wide)
	if !h {
		t.Fatal("wide doc must expose the horizontal zone")
	}
	// Mid-viewport clicks are never scrollbar clicks.
	if v, h := l.ScrollbarZones(l.OriginX+l.ViewW/2, l.OriginY+l.ViewH/2, tall); v || h {
		t.Fatalf("viewport centre is scrollbar zone: v=%v h=%v", v, h)
	}
}

func TestScrollbarZonesRespectTheHudBars(t *testing.T) {
	l := testLayout()
	tall := doc.New()
	tall.Load(strings.Repeat("row\n", 40))
	// Above the viewport (inside the tab strip area) is not a zone.
	if v, _ := l.ScrollbarZones(l.OriginX+l.ViewW-1, l.OriginY-1, tall); v {
		t.Fatal("the bar above the viewport must not be scrollbar territory")
	}
}
