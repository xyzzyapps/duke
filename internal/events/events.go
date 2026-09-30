// Package events defines the editor-wide event types and a small synchronous
// publish/subscribe bus.
//
// The bus decouples the platform shell (which polls Ebitengine's raw input)
// from the editor logic: the shell publishes input events, the UI editor
// subscribes and maps them onto engine commands. Handlers run synchronously
// on the caller's goroutine; Publish iterates over a snapshot so a handler
// may safely unsubscribe (itself or others) mid-dispatch.
package events

import "github.com/hajimehoshi/ebiten/v2"

// Event is the marker interface for every event on the bus.
type Event interface{ isEvent() }

// RuneTyped is one character typed by the user (already shifted/composed by
// the platform). Ctrl chords are never reported as runes.
type RuneTyped struct {
	Ch rune
}

func (RuneTyped) isEvent() {}

// KeyPressed is a discrete key press including its modifier state at the
// time of the press. The shell emits one event on key-down and then repeats
// it at an editor-like key-repeat interval while the key is held.
type KeyPressed struct {
	Key   ebiten.Key
	Ctrl  bool
	Shift bool
	Alt   bool
}

func (KeyPressed) isEvent() {}

// MousePressed is a mouse click in window coordinates.
type MousePressed struct {
	X, Y   int
	Button ebiten.MouseButton
}

func (MousePressed) isEvent() {}

// MouseMoved carries the cursor position every frame so menu hovers and
// dialog hit-testing stay responsive without polling.
type MouseMoved struct {
	X, Y int
}

func (MouseMoved) isEvent() {}

// QuitRequested asks the shell to exit cleanly (File > Close).
type QuitRequested struct{}

func (QuitRequested) isEvent() {}

// Handler receives published events.
type Handler func(Event)

// Bus is the pub/sub interface used everywhere in the editor; mocked in UI
// tests.
type Bus interface {
	// Subscribe registers h for all events and returns a cancel function
	// that is safe to call more than once.
	Subscribe(h Handler) (cancel func())
	// Publish delivers e to every current subscriber in subscription order.
	Publish(e Event)
}

type sub struct {
	id int
	h  Handler
}

type bus struct {
	subs []sub
	next int
}

// NewBus returns an empty synchronous bus.
func NewBus() Bus { return &bus{} }

// Subscribe implements Bus.
func (b *bus) Subscribe(h Handler) func() {
	b.next++
	id := b.next
	b.subs = append(b.subs, sub{id: id, h: h})
	return func() {
		for i := range b.subs {
			if b.subs[i].id == id {
				b.subs = append(b.subs[:i], b.subs[i+1:]...)
				return
			}
		}
	}
}

// Publish implements Bus.
func (b *bus) Publish(e Event) {
	if len(b.subs) == 0 {
		return
	}
	// Snapshot: handlers may mutate b.subs while we iterate.
	snap := make([]sub, len(b.subs))
	copy(snap, b.subs)
	for _, s := range snap {
		s.h(e)
	}
}
