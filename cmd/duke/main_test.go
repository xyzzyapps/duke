package main

import "testing"

func TestFitWindowToMonitor(t *testing.T) {
	// A big monitor: the nominal canvas is kept.
	if w, h := fitWindowToMonitor(1440, 1100, 2560, 1440); w != 1440 || h != 1100 {
		t.Fatalf("spacious monitor fit = %dx%d, want 1440x1100", w, h)
	}
	// 1080p with a taskbar: height-limited, aspect preserved.
	w, h := fitWindowToMonitor(1440, 1100, 1920, 1080)
	if w >= 1440 || h > 1080-96 {
		t.Fatalf("1080p fit = %dx%d, want both scaled down", w, h)
	}
	if a, b := float64(w)/float64(h), 1440.0/1100.0; a > b+0.01 || a < b-0.01 {
		t.Fatalf("1080p fit broke the aspect: %v vs %v", a, b)
	}
	// Wide-but-short (ultrawide): the aspect must survive.
	w2, h2 := fitWindowToMonitor(1440, 1100, 3440, 900)
	if r := float64(w2) / float64(h2); r < 1.29 || r > 1.33 {
		t.Fatalf("ultrawide fit broke the aspect: %dx%d (%v)", w2, h2, r)
	}
	// Degenerate monitor metrics: the nominal size is kept.
	if w3, h3 := fitWindowToMonitor(1440, 1100, 200, 100); w3 != 1440 || h3 != 1100 {
		t.Fatalf("degenerate screen fit = %dx%d, want nominal", w3, h3)
	}
	// Invalid monitor: unchanged.
	if w4, h4 := fitWindowToMonitor(1440, 1100, 0, 0); w4 != 1440 || h4 != 1100 {
		t.Fatalf("no monitor fit = %dx%d, want nominal", w4, h4)
	}
}
