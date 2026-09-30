// Package bot drives the LAN test client: a second gunman who wanders
// around the shared buffer, destroys text at random and trash-talks.
//
// Think is a pure decision function fed by an injected RNG, so tests can
// pin a seed and replay the bot's behaviour exactly.
package bot

import (
	"math/rand"

	"shooter/internal/doc"
)

// Kind classifies a bot action.
type Kind int

const (
	ActNothing Kind = iota
	ActWalk         // move somewhere random
	ActType         // append Text at the caret
	ActShoot        // pistol/katana a random glyph (ShootAt Pos)
	ActWord         // shotgun a word (Back = behind caret)
	ActLine         // rocket a line (Back = to line start)
	ActChat         // say Chat above his head
)

// Action is one decision. Fields are used depending on Kind.
type Action struct {
	Kind Kind
	Pos  doc.Pos // ActWalk / ActShoot target
	Text string  // ActType payload
	Back bool    // ActWord / ActLine direction
	Chat string  // ActChat line
}

// Words the bot types into the buffer.
var Words = []string{"lol ", "rekt ", "pwned ", "boom ", "duke ", "abc ", "hi ", "get owned "}

// Lines are the bot's trash talk (shown as a chat bubble).
var Lines = []string{
	"got'em",
	"too slow",
	"hail to the king, baby",
	"pwned",
	"your text is mine",
	"boom",
	"is that all?",
	"walk it off",
}

// Think rolls the next bot action against the current buffer so every
// target stays in bounds. An empty buffer always yields ActType (there is
// nothing to shoot yet).
func Think(d doc.Document, caret doc.Pos, r *rand.Rand) Action {
	if d.LineCount() == 0 || d.Text() == "" {
		return Action{Kind: ActType, Text: pick(r, Words)}
	}
	switch roll := r.Intn(100); {
	case roll < 30:
		// Destructive: shoot a random glyph.
		line := r.Intn(d.LineCount())
		n := d.RuneCount(line)
		if n == 0 {
			return Action{Kind: ActLine, Back: r.Intn(2) == 0}
		}
		return Action{Kind: ActShoot, Pos: doc.Pos{Line: line, Col: r.Intn(n)}}
	case roll < 50:
		return Action{Kind: ActWord, Back: r.Intn(2) == 0}
	case roll < 65:
		return Action{Kind: ActLine, Back: r.Intn(2) == 0}
	case roll < 85:
		return Action{Kind: ActType, Text: pick(r, Words)}
	case roll < 92:
		return Action{Kind: ActChat, Chat: pick(r, Lines)}
	default:
		line := r.Intn(d.LineCount())
		col := 0
		if n := d.RuneCount(line); n > 0 {
			col = r.Intn(n + 1)
		}
		return Action{Kind: ActWalk, Pos: doc.Pos{Line: line, Col: col}}
	}
}

// pick chooses a random element (non-empty slices only).
func pick(r *rand.Rand, xs []string) string {
	return xs[r.Intn(len(xs))]
}
