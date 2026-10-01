package render

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempSheet creates base/<name>/ with one solid-red idle.png so
// the renderer tests always have a valid sheet (sprites/ is a repo-root
// runtime folder, not visible from the package directory).
func writeTempSheet(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	f, err := os.Create(filepath.Join(dir, name, "idle.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return dir
}

func TestSetSheetSwitchesAndRejects(t *testing.T) {
	r, err := newFromSheetDir(writeTempSheet(t, "duke"))
	if err != nil {
		t.Fatal(err)
	}
	names := r.SheetNames()
	if len(names) < 1 || !strings.Contains(strings.Join(names, " "), "duke") {
		t.Fatalf("sheets = %v, want duke", names)
	}
	if r.ActiveSheet() != defaultSheet {
		t.Fatalf("active = %q, want %q", r.ActiveSheet(), defaultSheet)
	}
	if !r.SetSheet("duke") || r.ActiveSheet() != "duke" {
		t.Fatalf("switching to duke failed: active = %q", r.ActiveSheet())
	}
	if r.SetSheet("no-such-sheet") || r.ActiveSheet() != "duke" {
		t.Fatalf("unknown sheet must be rejected, active = %q", r.ActiveSheet())
	}
}

func TestDiskSheetLoadsWithFallback(t *testing.T) {
	dir := t.TempDir()
	// A 4x4 solid-red idle frame: the only user PNG.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "test"), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "test", "idle.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	sheets, err := loadDiskSheetsFrom(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := sheets["test"]
	if !ok {
		t.Fatalf("sheets = %v, want test", sheets)
	}
	// The idle frame came from the PNG (resized to the art grid, red).
	if c := s.raw[frameIdle].RGBAAt(0, 0); c.R != 255 || c.G != 0 || c.B != 0 {
		t.Fatalf("idle pixel = %+v, want red", c)
	}
	if b := s.raw[frameIdle].Bounds(); b.Dx() != artW || b.Dy() != artH {
		t.Fatalf("idle bounds = %v, want %dx%d", b, artW, artH)
	}
	// Missing poses become the neutral placeholder box.
	got := s.raw[frameWalk0]
	if got == nil || got.Bounds().Dx() != artW || got.Bounds().Dy() != artH {
		t.Fatalf("walk0 placeholder missing or wrong size")
	}
	// The missing pose is the neutral placeholder box (there is no
	// built-in art to mix in any more).
	want := placeholderFrame()
	if got.RGBAAt(0, 0) != want.RGBAAt(0, 0) || got.RGBAAt(artW/2, artH/2) != want.RGBAAt(artW/2, artH/2) {
		t.Fatal("missing pose must be the placeholder box, not other art")
	}
}

func TestTruncateChatToOneBubbleLine(t *testing.T) {
	if got := truncateChat("short"); got != "short" {
		t.Fatalf("short chat = %q", got)
	}
	long := truncateChat(strings.Repeat("x", 40))
	if len(long) != 34 || long[33] != '~' {
		t.Fatalf("long chat = %q (want 34 chars ending in ~)", long)
	}
	if truncateChat("") != "" {
		t.Fatal("empty chat must stay empty")
	}
}
