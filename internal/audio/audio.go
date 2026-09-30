// Package audio plays the editor sound effects.
//
// The default engine loads WAV samples from the SoundsDir directory (one
// file per cue, see sampleNames). Missing files mean that cue stays
// silent, and with no files at all the audio device is never touched.
// The built-in procedural synthesizer is kept as an option for the future
// (the -synth flag): it renders each cue in memory, but it is NOT the
// default.
//
// All playback degrades safely: a nil Synth, a muted Synth or a vanished
// device never panic.
package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"shooter/internal/fx"
)

// SampleRate is the playback rate (Ebitengine's usual CD rate).
const SampleRate = 44100

// SoundsDir is where sample WAV files live (relative to the working
// directory). File names per cue: see sampleNames.
const SoundsDir = "sounds"

// sampleNames maps each cue to its WAV file name inside SoundsDir.
var sampleNames = map[fx.Sound]string{
	fx.SoundSlash:   "slash.wav",
	fx.SoundPistol:  "pistol.wav",
	fx.SoundShotgun: "shotgun.wav",
	fx.SoundRocket:  "rocket.wav",
	fx.SoundStamp:   "stamp.wav",
	fx.SoundSave:    "save.wav",
}

// Synth plays the pre-synthesised cues. All methods are safe on a nil
// Synth (the shell disables sound when construction fails) and safe to
// call from the game loop.
type Synth struct {
	players map[fx.Sound]*audio.Player
	muted   bool
	synth   bool // true when the procedural engine is active
	loaded  int  // samples successfully loaded
}

// The process owns exactly one audio context (a second NewContext panics
// in oto); every engine shares it.
var (
	ctxOnce sync.Once
	shared  *audio.Context
)

// sharedContext returns the process-wide audio context, or nil when no
// device could be opened (headless machines stay silent).
func sharedContext() *audio.Context {
	ctxOnce.Do(func() {
		defer func() {
			if recover() != nil {
				shared = nil
			}
		}()
		shared = audio.NewContext(SampleRate)
	})
	return shared
}

// NewSamples loads WAV samples from dir (normally SoundsDir). Cues whose
// files are missing stay silent; when nothing loads, the audio device is
// never opened. Never fails.
func NewSamples(dir string) (s *Synth) {
	// Named return: a recover() inside the deferred handler must keep s
	// usable (a bare return after recover yields nil otherwise).
	s = &Synth{players: map[fx.Sound]*audio.Player{}}
	// Open every file first: only touch the audio device if we have
	// something to play.
	type pending struct {
		snd  fx.Sound
		data []byte
	}
	var files []pending
	for _, snd := range allSounds() {
		data, err := os.ReadFile(filepath.Join(dir, sampleNames[snd]))
		if err != nil {
			continue
		}
		files = append(files, pending{snd, data})
	}
	if len(files) == 0 {
		return s // fully silent: no device, no work
	}
	ctx := sharedContext()
	if ctx == nil {
		return s
	}
	// Decode from memory: file handles are released immediately (an open
	// handle would make Windows refuse to clean up test fixture dirs).
	for _, p := range files {
		stream, err := wav.Decode(ctx, bytes.NewReader(p.data))
		if err != nil {
			continue
		}
		pl, err := ctx.NewPlayer(stream)
		if err != nil {
			continue
		}
		s.players[p.snd] = pl
		s.loaded++
	}
	return s
}

// NewSynth renders every cue with the built-in procedural synthesizer
// (the optional/future engine selected by the -synth flag). Never fails.
func NewSynth() (s *Synth) {
	s = &Synth{players: map[fx.Sound]*audio.Player{}, synth: true}
	ctx := sharedContext()
	if ctx == nil {
		return s
	}
	for _, snd := range allSounds() {
		pl, err := ctx.NewPlayer(bytes.NewReader(pcm(snd)))
		if err != nil {
			continue
		}
		s.players[snd] = pl
		s.loaded++
	}
	return s
}

// Mode describes the active engine for a startup log line.
func (s *Synth) Mode() string {
	switch {
	case s == nil:
		return "off"
	case s.synth:
		return "synth (built-in synthesizer)"
	case s.loaded == 0:
		return "silent (no WAV samples in " + SoundsDir + ")"
	default:
		return fmt.Sprintf("samples (%d WAV files from %s)", s.loaded, SoundsDir)
	}
}

// allSounds lists every cue (kept explicit so a new fx.Sound without a
// synth shows up as a missing player rather than a silent bug).
func allSounds() []fx.Sound {
	return []fx.Sound{
		fx.SoundSlash,
		fx.SoundPistol,
		fx.SoundShotgun,
		fx.SoundRocket,
		fx.SoundStamp,
		fx.SoundSave,
	}
}

// Play replays a cue from the start (no-op while muted, for unknown cues
// or on a nil Synth).
func (s *Synth) Play(snd fx.Sound) {
	if s == nil || s.muted {
		return
	}
	p, ok := s.players[snd]
	if !ok {
		return
	}
	defer func() { _ = recover() }() // device may vanish mid-session
	_ = p.Rewind()
	p.Play()
}

// ToggleMute flips muting and reports the new state.
func (s *Synth) ToggleMute() bool {
	if s == nil {
		return true
	}
	s.muted = !s.muted
	return s.muted
}

// Muted reports the mute state.
func (s *Synth) Muted() bool {
	return s == nil || s.muted
}

// pcm renders the raw s16le mono samples for a cue.
func pcm(snd fx.Sound) []byte {
	switch snd {
	case fx.SoundSlash:
		return render(0.20, whoosh)
	case fx.SoundPistol:
		return render(0.20, crack)
	case fx.SoundShotgun:
		return render(0.40, boom)
	case fx.SoundRocket:
		return render(0.32, launch)
	case fx.SoundStamp:
		return render(0.05, tick)
	case fx.SoundSave:
		return render(0.28, chime)
	}
	return render(0.01, func(float64) float64 { return 0 })
}

// render converts a sample function into s16le PCM bytes.
func render(seconds float64, f func(t float64) float64) []byte {
	n := int(seconds * SampleRate)
	out := make([]byte, n*2)
	for i := 0; i < n; i++ {
		v := f(float64(i) / SampleRate)
		var s int16
		switch {
		case v >= 1:
			s = 32767
		case v <= -1:
			s = -32768
		default:
			s = int16(v * 32768)
		}
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
	}
	return out
}

// noise returns white noise in [-1, 1].
func noise() float64 { return rand.Float64()*2 - 1 }

// whoosh is the katana cutting the air: band-passed-feeling noise with a
// swell that peaks mid-swing (matches the 0.10..0.20s cut window).
func whoosh(t float64) float64 {
	if t > 0.20 {
		return 0
	}
	swell := math.Sin(math.Pi * t / 0.20)
	// Squaring the swell sharpens the peak; scale the noise by a slow
	// sine so it breathes instead of hissing flat.
	return noise() * swell * swell * 0.55
}

// crack is the pistol: a hard transient click plus a decaying low thump.
func crack(t float64) float64 {
	env := math.Exp(-t * 34)
	body := noise() * env * 0.85
	thump := math.Sin(2*math.Pi*170*t) * math.Exp(-t*26) * 0.45
	// Initial transient: a few milliseconds of full-amplitude noise.
	transient := 0.0
	if t < 0.006 {
		transient = noise() * 0.9
	}
	return body + thump + transient
}

// boom is the shotgun (and the rocket's impact): deep and long.
func boom(t float64) float64 {
	rumble := noise() * math.Exp(-t*13) * 0.7
	low := math.Sin(2*math.Pi*66*t) * math.Exp(-t*8) * 0.75
	transient := 0.0
	if t < 0.01 {
		transient = noise() * 0.8
	}
	return rumble + low + transient
}

// launch is the rocket leaving the tube: a rising whistle over thrust.
func launch(t float64) float64 {
	freq := 140 + 700*t // sweep upward
	whistle := math.Sin(2*math.Pi*freq*t) * math.Exp(-t*7) * 0.5
	thrust := noise() * math.Exp(-t*9) * 0.35
	return whistle + thrust
}

// tick is a thrown letter locking into the buffer.
func tick(t float64) float64 {
	freq := 900 - 500*t/0.05
	return math.Sin(2*math.Pi*freq*t) * math.Exp(-t*70) * 0.4
}

// chime is the save jingle: two ascending notes.
func chime(t float64) float64 {
	const (
		n1End = 0.11
		n2Beg = 0.13
	)
	switch {
	case t < n1End:
		return math.Sin(2*math.Pi*660*t) * math.Exp(-t*9) * 0.4
	case t >= n2Beg:
		return math.Sin(2*math.Pi*880*(t-n2Beg)) * math.Exp(-(t-n2Beg)*7) * 0.4
	}
	return 0
}

// GenerateSamples writes one WAV sample per cue into dir (creating it).
// This is OFFLINE asset generation (cmd/gensounds): the runtime never
// synthesises - it only ever plays files.
func GenerateSamples(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("gensounds: %w", err)
	}
	for _, snd := range allSounds() {
		path := filepath.Join(dir, sampleNames[snd])
		if err := os.WriteFile(path, wavWrap(pcm(snd)), 0o644); err != nil {
			return fmt.Errorf("gensounds %s: %w", path, err)
		}
	}
	return nil
}

// wavWrap frames raw s16le mono PCM in a canonical RIFF/WAVE header.
func wavWrap(pcmData []byte) []byte {
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

// Cues lists every sound cue in default play order (used by tests and
// the -playsounds audition mode).
func Cues() []fx.Sound { return allSounds() }
