package render

import (
	"image"
	"image/color"
	"testing"

	"shooter/internal/agent"
)

// TestSpriteGeometryMatchesAgent pins the art size to the agent's logical
// sprite size through the font scale: art * Scale == SpriteW x SpriteH.
func TestSpriteGeometryMatchesAgent(t *testing.T) {
	if artW*Scale != int(agent.SpriteW) {
		t.Fatalf("artW*Scale = %v, agent.SpriteW = %v", artW*Scale, agent.SpriteW)
	}
	if artH*Scale != int(agent.SpriteH) {
		t.Fatalf("artH*Scale = %v, agent.SpriteH = %v", artH*Scale, agent.SpriteH)
	}
	// The sprite must not be taller than a layout row, or the gunman would
	// clip into the neighbouring line (or under the HUD at line 0).
	if int(agent.SpriteH) != cellHForTest() {
		t.Fatalf("sprite height %v != cell height %v", agent.SpriteH, cellHForTest())
	}
}

// cellHForTest mirrors the runtime cell height: the art sheet at Scale.
func cellHForTest() int { return artH * Scale }

func TestPoseMapping(t *testing.T) {
	s := &spriteSheet{}
	cases := []struct {
		snap agent.Snapshot
		want frameID
		xOff int
		yOff int
	}{
		{agent.Snapshot{State: agent.StateIdle}, frameIdle, 0, 0},
		{agent.Snapshot{State: agent.StateWalk, Frame: 1}, frameWalk1, 0, -1}, // passing rides up
		{agent.Snapshot{State: agent.StateWalk, Frame: 5}, frameWalk1, 0, -1}, // wraps
		{agent.Snapshot{State: agent.StateAim}, frameAim, 0, 0},
		{agent.Snapshot{State: agent.StateFire}, frameAim, 0, 0},
		{agent.Snapshot{State: agent.StateRecoil}, frameAim, 1, 0},             // kick back (+x = his back)
		{agent.Snapshot{State: agent.StateSlash, T: 0}, frameKatanaDraw, 0, 0}, // reaching for the sheath
		{agent.Snapshot{State: agent.StateSlash, T: 0.2}, frameKatana, 0, 0},   // both hands on the hilt
		{agent.Snapshot{State: agent.StateDrag}, frameKatana, 0, 0},            // hands, not gun
		{agent.Snapshot{State: agent.StateWin}, frameWin, 0, -2},
	}
	for i, c := range cases {
		id, xOff, yOff := s.pose(c.snap)
		if id != c.want || xOff != c.xOff || yOff != c.yOff {
			t.Fatalf("case %d: got (%d,%d,%d), want (%d,%d,%d)",
				i, id, xOff, yOff, c.want, c.xOff, c.yOff)
		}
	}
}

// TestPlaceholderFillsMissingPoses pins the disk-loader contract: a pose
// without a PNG gets the neutral placeholder box, never other art.
func TestPlaceholderFillsMissingPoses(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	// (The full disk-loading flow lives in render_test.go; this pins the
	// placeholder itself.)
	p := placeholderFrame()
	if b := p.Bounds(); b.Dx() != artW || b.Dy() != artH {
		t.Fatalf("placeholder bounds = %v, want %dx%d", b, artW, artH)
	}
	if c := p.RGBAAt(artW/2, artH/2); c.A != 255 {
		t.Fatalf("placeholder fill must be opaque, got %+v", c)
	}
	if c := p.RGBAAt(0, 0); c.A == 0 {
		t.Fatal("placeholder border must be visible")
	}
}
