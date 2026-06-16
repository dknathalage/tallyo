package agent

import "testing"

func TestEventsSubscribePublishUnsubscribe(t *testing.T) {
	e := NewEvents()
	ch, unsub := e.Subscribe(7)

	// An event for another conversation is not delivered.
	e.Publish(8, Event{Type: "noise"})
	// An event for this conversation is delivered.
	e.Publish(7, Event{Type: "plan", Data: 1})

	select {
	case ev := <-ch:
		if ev.Type != "plan" {
			t.Fatalf("got %q, want plan", ev.Type)
		}
	default:
		t.Fatal("expected an event on the channel")
	}

	// After unsubscribe the channel is closed and publishing is a no-op.
	unsub()
	unsub() // idempotent
	if _, open := <-ch; open {
		t.Fatal("channel should be closed after unsubscribe")
	}
	e.Publish(7, Event{Type: "late"}) // must not panic
}

func TestEventsNonBlockingOnFullBuffer(t *testing.T) {
	e := NewEvents()
	_, unsub := e.Subscribe(1)
	defer unsub()
	// Publish more than the buffer can hold; must not block.
	for i := 0; i < eventBufferSize*3; i++ { // bounded
		e.Publish(1, Event{Type: "x", Data: i})
	}
}
