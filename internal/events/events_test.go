package events

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestPublishReachesSubscribersInOrder(t *testing.T) {
	b := NewBus()
	var got []int
	b.Subscribe(func(Event) { got = append(got, 1) })
	b.Subscribe(func(Event) { got = append(got, 2) })
	b.Publish(RuneTyped{Ch: 'a'})
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("got = %v, want [1 2]", got)
	}
}

func TestEventPayloadArrives(t *testing.T) {
	b := NewBus()
	var ch rune
	var key ebiten.Key
	b.Subscribe(func(e Event) {
		switch ev := e.(type) {
		case RuneTyped:
			ch = ev.Ch
		case KeyPressed:
			key = ev.Key
		}
	})
	b.Publish(RuneTyped{Ch: 'Z'})
	b.Publish(KeyPressed{Key: ebiten.KeyLeft, Ctrl: true})
	if ch != 'Z' {
		t.Fatalf("ch = %q, want Z", ch)
	}
	if key != ebiten.KeyLeft {
		t.Fatalf("key = %v, want Left", key)
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	b := NewBus()
	calls := 0
	cancel := b.Subscribe(func(Event) { calls++ })
	b.Publish(RuneTyped{Ch: 'a'})
	cancel()
	cancel() // double cancel must be harmless
	b.Publish(RuneTyped{Ch: 'b'})
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestUnsubscribeDuringPublishIsSafe(t *testing.T) {
	b := NewBus()
	var cancelFirst func()
	callsSecond := 0
	cancelFirst = b.Subscribe(func(Event) { cancelFirst() })
	b.Subscribe(func(Event) { callsSecond++ })
	// Cancelling the first subscriber while it is being dispatched must
	// neither panic nor skip the second subscriber.
	b.Publish(RuneTyped{Ch: 'x'})
	b.Publish(RuneTyped{Ch: 'y'})
	if callsSecond != 2 {
		t.Fatalf("callsSecond = %d, want 2", callsSecond)
	}
}

func TestPublishWithNoSubscribersIsNoop(t *testing.T) {
	b := NewBus()
	b.Publish(KeyPressed{Key: ebiten.KeyEnter}) // must not panic
}
