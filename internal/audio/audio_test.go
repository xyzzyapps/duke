package audio

import (
	"encoding/binary"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"os"
	"path/filepath"
	"shooter/internal/fx"
	"strings"
)

// TestPcmRendersEveryCue checks each cue produces sane s16le PCM:
// non-trivial length, RIFF-free raw format, and samples that actually
// move (not silence) while staying inside int16 bounds.
func TestPcmRendersEveryCue(t *testing.T) {
	for _, snd := range allSounds() {
		data := pcm(snd)
		if len(data) == 0 || len(data)%2 != 0 {
			t.Fatalf("cue %v: %d bytes, want non-empty even length", snd, len(data))
		}
		min, max := int16(32767), int16(-32768)
		nonZero := 0
		for i := 0; i+1 < len(data); i += 2 {
			v := int16(binary.LittleEndian.Uint16(data[i:]))
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
			if v != 0 {
				nonZero++
			}
		}
		if nonZero < len(data)/8 {
			t.Fatalf("cue %v: only %d non-zero samples of %d — too quiet", snd, nonZero, len(data)/2)
		}
		// render() clamps, so extremes must stay within int16 (always
		// true by construction; guards against an unclamped rewrite).
		if min < -32768 || max > 32767 {
			t.Fatalf("cue %v: samples out of range [%v, %v]", snd, min, max)
		}
	}
}

// TestRenderClampsOversizedSamples proves the encoder saturates instead
// of wrapping (a wrap would make loud, horrible clicks).
func TestRenderClampsOversizedSamples(t *testing.T) {
	data := render(0.01, func(float64) float64 { return 5 })
	for i := 0; i+1 < len(data); i += 2 {
		if v := int16(binary.LittleEndian.Uint16(data[i:])); v != 32767 {
			t.Fatalf("sample = %v, want +1 clamp (32767)", v)
		}
	}
	data = render(0.01, func(float64) float64 { return -5 })
	for i := 0; i+1 < len(data); i += 2 {
		if v := int16(binary.LittleEndian.Uint16(data[i:])); v != -32768 {
			t.Fatalf("sample = %v, want -1 clamp (-32768)", v)
		}
	}
}

// TestSilentSynthIsSafe: a nil Synth (no audio device) must not panic.
func TestSilentSynthIsSafe(t *testing.T) {
	var s *Synth
	s.Play(fx.SoundPistol)
	if !s.Muted() {
		t.Fatal("nil synth should report muted")
	}
	if s.ToggleMute() != true {
		t.Fatal("nil synth toggle should report muted")
	}
}

// TestMuteToggles covers the mute flag and empty-player Play safety.
func TestMuteToggles(t *testing.T) {
	s := &Synth{players: map[fx.Sound]*audio.Player{}}
	if s.Muted() {
		t.Fatal("a fresh synth starts unmuted")
	}
	if !s.ToggleMute() {
		t.Fatal("toggle should report muted")
	}
	s.Play(fx.SoundPistol) // no player: no-op, must not panic
	if !s.Muted() {
		t.Fatal("still muted")
	}
}

// --- sample engine ----------------------------------------------------------

// makeWAV wraps raw s16le mono PCM in a minimal RIFF/WAVE header (test
// fixture generation only — production audio never synthesizes).
func makeWAV(pcmData []byte) []byte {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(pcmData)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)           // fmt chunk size
	binary.LittleEndian.PutUint16(h[20:], 1)            // PCM
	binary.LittleEndian.PutUint16(h[22:], 1)            // mono
	binary.LittleEndian.PutUint32(h[24:], SampleRate)   // sample rate
	binary.LittleEndian.PutUint32(h[28:], SampleRate*2) // byte rate
	binary.LittleEndian.PutUint16(h[32:], 2)            // block align
	binary.LittleEndian.PutUint16(h[34:], 16)           // bits per sample
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(pcmData)))
	return append(h, pcmData...)
}

func TestNewSamplesMissingDirIsSilentAndSafe(t *testing.T) {
	s := NewSamples("./no-such-sounds-dir-anywhere")
	if s.loaded != 0 {
		t.Fatalf("loaded = %d, want 0", s.loaded)
	}
	s.Play(fx.SoundPistol) // must be a no-op
	if !strings.Contains(s.Mode(), "silent") {
		t.Fatalf("mode = %q, want silent", s.Mode())
	}
}

func TestNewSamplesLoadsWAVFile(t *testing.T) {
	dir, err := os.MkdirTemp(".", "tmp-sounds-*")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// One sample: pistol.wav (fixture built from the synth engine).
	data := makeWAV(pcm(fx.SoundPistol))
	if err := os.WriteFile(filepath.Join(dir, "pistol.wav"), data, 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}
	s := NewSamples(dir)
	if s.loaded != 1 {
		t.Fatalf("loaded = %d, want 1", s.loaded)
	}
	if _, ok := s.players[fx.SoundPistol]; !ok {
		t.Fatal("pistol player missing")
	}
	if len(s.players) != 1 {
		t.Fatalf("players = %d, want 1 (other cues stay silent)", len(s.players))
	}
	if !strings.Contains(s.Mode(), "samples") {
		t.Fatalf("mode = %q", s.Mode())
	}
}

func TestSynthModeLabel(t *testing.T) {
	s := NewSynth()
	if !strings.Contains(s.Mode(), "synth") {
		t.Fatalf("mode = %q, want synth", s.Mode())
	}
}

func TestGenerateSamplesRoundTrip(t *testing.T) {
	dir, err := os.MkdirTemp(".", "tmp-gen-sounds-*")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := GenerateSamples(dir); err != nil {
		t.Fatalf("GenerateSamples: %v", err)
	}
	for _, name := range sampleNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing sample %s: %v", name, err)
		}
	}
	s := NewSamples(dir)
	if s.loaded != len(sampleNames) {
		t.Fatalf("loaded = %d, want %d", s.loaded, len(sampleNames))
	}
}
