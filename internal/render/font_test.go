package render

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"shooter/internal/agent"
	"shooter/internal/fonts"
)

// Face used by Renderer.New — rebuilt here so the metrics can be pinned
// without a full renderer.
func newBufFaceForTest(t *testing.T, size float64) *text.GoTextFace {
	t.Helper()
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.JetBrainsMonoTTF))
	if err != nil {
		t.Fatalf("face source: %v", err)
	}
	return &text.GoTextFace{Source: src, Size: size}
}

// TestFontAdvanceMatchesCell pins the grid contract: JetBrains Mono advances
// 0.6em, so at 30px the document face is exactly one cell wide per
// glyph and every column lines up with the sprite grid.
func TestFontAdvanceMatchesCell(t *testing.T) {
	face := newBufFaceForTest(t, 30)
	adv := text.Advance("M", face)
	if math.Round(adv) != float64(artW/2*Scale) {
		t.Fatalf("M advance = %v, want cell width %d", adv, artW/2*Scale)
	}
	// The document face must fit inside the cell height (the sprite owns
	// it), or glyphs would clip into the neighbouring line.
	m := face.Metrics()
	if line := m.HAscent + m.HDescent; line > float64(artH*Scale) {
		t.Fatalf("document line height %v taller than a cell %d", line, artH*Scale)
	}
}

// TestUtf8ColumnsStayAligned: every rune — ASCII, accents, Greek, Cyrillic
// and a CJK placeholder — advances a full column, so UTF-8 text can never
// drift the editor grid out of alignment.
func TestUtf8ColumnsStayAligned(t *testing.T) {
	face := newBufFaceForTest(t, 30)
	cases := []string{
		"a",       // ASCII
		"é",       // accented Latin
		"Ω",       // Greek
		"Ж",       // Cyrillic
		"中",       // CJK (placeholder glyph in Go fonts)
		"e\u0301", // combining accent (two runes, one column each)
		"héllo wörld Ω",
	}
	for _, s := range cases {
		n := len([]rune(s))
		got := text.Advance(s, face)
		want := float64(n) * float64(artW/2*Scale)
		if strings.ContainsRune(s, '\u0301') {
			// Combining marks are zero-width (they overlay the base glyph).
			want = float64(n-1) * float64(artW/2*Scale)
		}
		if math.Abs(got-want) > 1.5 {
			t.Fatalf("Advance(%q) = %v, want %v (%d columns)", s, got, want, n)
		}
	}
}

// TestKatanaPivotStartsAtTheBackAndEndsInFront pins the draw-from-the-sheath
// motion: at t=0 the pivot is on the back (opposite the eyes), by t>=0.14 it
// has slid to his front hands, so the blade points at the struck glyph.
func TestKatanaPivotStartsAtTheBackAndEndsInFront(t *testing.T) {
	center := func(x float64) float64 { return x + agent.SpriteW/2 }

	cases := []struct {
		facing int
		t      float64
		back   bool // true when the pivot must still be on the back side
	}{
		{1, 0.0, true},
		{-1, 0.0, true},
		{1, 0.05, true},
		{-1, 0.05, true},
		{1, 0.14, false},
		{-1, 0.14, false},
		{1, 0.3, false},
		{-1, 0.3, false},
	}
	for _, c := range cases {
		a := agent.Snapshot{X: 100, Y: 200, Facing: c.facing, T: c.t}
		px := katanaPivotX(a, 0)
		// Facing right: back = left of centre; facing left: back = right.
		onBack := (c.facing == 1 && px < center(a.X)) || (c.facing == -1 && px > center(a.X))
		if onBack != c.back {
			t.Fatalf("facing %d t=%v: pivot %v %s of centre (want %s)",
				c.facing, c.t, px, map[bool]string{true: "back", false: "front"}[onBack],
				map[bool]string{true: "back", false: "front"}[c.back])
		}
	}
	// The pivot also travels from one side to the other, never jumping.
	a := agent.Snapshot{X: 100, Y: 200, Facing: 1, T: 0.02}
	p0 := katanaPivotX(a, 0)
	a.T = 0.13
	p1 := katanaPivotX(a, 0)
	if p1 <= p0 {
		t.Fatalf("pivot must slide forward: %v -> %v", p0, p1)
	}
}

func TestSetFontSizeRebuildsTheGrid(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if r.FontSize() != DefaultFontSize {
		t.Fatalf("default size = %d, want %d", r.FontSize(), DefaultFontSize)
	}
	if r.Layout().Rows() != 20 || r.Layout().Cols() != 80 {
		t.Fatalf("default grid = %dx%d cells, want 80x20", r.Layout().Cols(), r.Layout().Rows())
	}
	if r.Layout().ScreenW != 1440 || r.Layout().ScreenH != 1100 {
		t.Fatalf("default window = %dx%d, want 1440x1100", r.Layout().ScreenW, r.Layout().ScreenH)
	}
	// 18px: advance 10.8 -> cellW 11, window 880 wide; rows stay 20.
	if !r.SetFontSize(18) {
		t.Fatal("size 18 rejected")
	}
	if r.Layout().CellW != 11 || r.Layout().ScreenW != 880 {
		t.Fatalf("18px grid cellW = %d (window %d), want 11 (880)", r.Layout().CellW, r.Layout().ScreenW)
	}
	if r.Layout().Rows() != 20 || r.Layout().ScreenH != 1100 {
		t.Fatalf("18px rows/window height = %d/%d, want 20/1100", r.Layout().Rows(), r.Layout().ScreenH)
	}
	// 24px: advance 14.4 -> cellW 14.
	if !r.SetFontSize(24) {
		t.Fatal("size 24 rejected")
	}
	if r.Layout().CellW != 14 || r.Layout().ScreenW != 1120 {
		t.Fatalf("24px grid cellW = %d (window %d), want 14 (1120)", r.Layout().CellW, r.Layout().ScreenW)
	}
	// Invalid sizes are rejected; back to 30 restores 1440.
	if r.SetFontSize(99) {
		t.Fatal("size 99 must be rejected")
	}
	if !r.SetFontSize(30) || r.Layout().ScreenW != 1440 {
		t.Fatal("back to 30 must restore the 1440px window")
	}
	if got, cur := r.FontSizeList(); len(got) != 3 || cur != 30 {
		t.Fatalf("FontSizeList = %v/%d, want [18 24 30]/30", got, cur)
	}
}
