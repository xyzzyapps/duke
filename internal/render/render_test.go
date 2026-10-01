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

func TestSetSheetSwitchesAndRejects(t *testing.T) {
	r, err := New()
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
	// Missing poses fall back to the built-in duke art.
	want, err := parseFrame(frameArt[frameWalk0])
	if err != nil {
		t.Fatal(err)
	}
	got := s.raw[frameWalk0]
	if got == nil || got.Bounds() != want.Bounds() {
		t.Fatalf("walk0 fallback missing or wrong size")
	}
	if color.RGBAModel.Convert(got.At(2, 2)) != color.RGBAModel.Convert(want.At(2, 2)) {
		t.Fatalf("walk0 fallback differs from the built-in frame")
	}
}
