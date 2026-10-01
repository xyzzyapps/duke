package render

import (
	"testing"

	"shooter/internal/agent"
)

func TestAllFramesParse(t *testing.T) {
	for id := frameID(0); id < frameCount; id++ {
		rows, ok := frameArt[id]
		if !ok {
			t.Fatalf("frame %d has no art", id)
		}
		img, err := parseFrame(rows)
		if err != nil {
			t.Fatalf("frame %d: %v", id, err)
		}
		if img.Bounds().Dx() != artW || img.Bounds().Dy() != artH {
			t.Fatalf("frame %d bounds = %v", id, img.Bounds())
		}
	}
}

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

func TestFrameArtRejectsBadRows(t *testing.T) {
	if _, err := parseFrame([]string{"too", "short"}); err == nil {
		t.Fatal("parseFrame must reject wrong row counts")
	}
	bad := make([]string, artH)
	for i := range bad {
		bad[i] = "............" //12 dots
	}
	bad[0] = "............X" // 13 columns
	if _, err := parseFrame(bad); err == nil {
		t.Fatal("parseFrame must reject wrong column counts")
	}
	bad[0] = "....Z......." // unknown palette char
	if _, err := parseFrame(bad); err == nil {
		t.Fatal("parseFrame must reject unknown palette chars")
	}
}

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
		{agent.Snapshot{State: agent.StateSlash, T: 0}, frameKatanaDraw, 0, 0}, // reaching for the sheath\n		{agent.Snapshot{State: agent.StateSlash, T: 0.2}, frameKatana, 0, 0},       // both hands on the hilt
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

// TestWalkCycleAlternatesFeet pins the four-beat gait: contact A and
// contact B must differ (lifted foot swaps sides) while the passing
// frames reuse the legs-together pose.
func TestWalkCycleAlternatesFeet(t *testing.T) {
	same := func(a, b frameID) bool {
		x, y := frameArt[a], frameArt[b]
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}
		return true
	}
	if same(frameWalk0, frameWalk2) {
		t.Fatal("walk0 and walk2 must differ: the lifted foot swaps sides")
	}
	if !same(frameWalk1, frameWalk3) {
		t.Fatal("walk1 and walk3 are the same passing pose")
	}
	// The lifted foot has no sole under it on the contact frames.
	if frameArt[frameWalk0][23] != ".kkkkkkk.........." {
		t.Fatalf("walk0 sole row = %q", frameArt[frameWalk0][23])
	}
	if frameArt[frameWalk2][23] != "...........kkkkkkk" {
		t.Fatalf("walk2 sole row = %q", frameArt[frameWalk2][23])
	}
}

// TestMouthIsTwoDarkPixels pins the simple mouth: two dark pixels in one
// straight row, in both the idle and victory frames.
func TestMouthIsTwoDarkPixels(t *testing.T) {
	const mouthRow = 7 // empty row + headRows[6] (idle and win line up)
	if got := frameArt[frameIdle][mouthRow]; got != "..kssssskkkssssk.." {
		t.Fatalf("idle mouth row = %q, want %q", got, "..kssssskkkssssk..")
	}
	if got := frameArt[frameWin][mouthRow]; got != "..kssssskkkssssk.." {
		t.Fatalf("win mouth row = %q, want %q", got, "..kssssskkkssssk..")
	}
}
