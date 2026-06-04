package realtime

import (
	"sync"
	"testing"
	"time"
)

func TestSubscribeReceivesBroadcast(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe()
	defer unsub()

	h.Broadcast(Event{Entity: "invoice", ID: 1, Action: "update"})

	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.ID != 1 || e.Action != "update" {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe()
	unsub()
	h.Broadcast(Event{Entity: "x", ID: 1, Action: "update"})
	// channel should be closed (unsub closes it) or no value delivered
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("should not receive after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		// acceptable: nothing delivered
	}
}

func TestBroadcastDoesNotBlockOnSlowClient(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe() // never read from ch
	defer unsub()
	done := make(chan struct{})
	go func() {
		// broadcast many more than the buffer cap; must not block
		for i := 0; i < 1000; i++ {
			h.Broadcast(Event{Entity: "e", ID: int64(i), Action: "update"})
		}
		close(done)
	}()
	select {
	case <-done:
		// ok — broadcaster never blocked
	case <-time.After(2 * time.Second):
		t.Fatal("Broadcast blocked on slow client")
	}
	_ = ch
}

func TestConcurrentSubscribeBroadcast(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, unsub := h.Subscribe()
			defer unsub()
			for j := 0; j < 50; j++ {
				h.Broadcast(Event{Entity: "e", ID: int64(j), Action: "update"})
				select {
				case <-ch:
				default:
				}
			}
		}()
	}
	wg.Wait()
}
