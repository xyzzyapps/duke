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

// cellHForTest mirrors the runtime cell height: (ascent 12 + descent 4) * Scale.
func cellHForTest() int { return 16 * Scale }

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
		{agent.Snapshot{State: agent.StateWalk, Frame: 1}, frameWalk1, 0, 0},
		{agent.Snapshot{State: agent.StateWalk, Frame: 5}, frameWalk1, 0, 0}, // wraps
		{agent.Snapshot{State: agent.StateAim}, frameAim, 0, 0},
		{agent.Snapshot{State: agent.StateFire}, frameAim, 0, 0},
		{agent.Snapshot{State: agent.StateRecoil}, frameAim, -1, 0},
		{agent.Snapshot{State: agent.StateSlash}, frameKatana, 0, 0},
		{agent.Snapshot{State: agent.StateDrag}, frameKatana, 0, 0}, // hands, not gun
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
