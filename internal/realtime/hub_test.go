package realtime

import (
	"sync"
	"testing"
	"time"
)

const testTenant int64 = 1

func TestSubscribeReceivesBroadcast(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(testTenant)
	defer unsub()

	h.Broadcast(Event{TenantID: testTenant, Entity: "invoice", ID: 1, Action: "update"})

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
	ch, unsub := h.Subscribe(testTenant)
	unsub()
	h.Broadcast(Event{TenantID: testTenant, Entity: "x", ID: 1, Action: "update"})
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
	ch, unsub := h.Subscribe(testTenant) // never read from ch
	defer unsub()
	done := make(chan struct{})
	go func() {
		// broadcast many more than the buffer cap; must not block
		for i := 0; i < 1000; i++ {
			h.Broadcast(Event{TenantID: testTenant, Entity: "e", ID: int64(i), Action: "update"})
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
		go func(tid int64) {
			defer wg.Done()
			ch, unsub := h.Subscribe(tid)
			defer unsub()
			for j := 0; j < 50; j++ {
				h.Broadcast(Event{TenantID: tid, Entity: "e", ID: int64(j), Action: "update"})
				select {
				case <-ch:
				default:
				}
			}
		}(int64(i%3) + 1)
	}
	wg.Wait()
}

// TestEventsDoNotLeakAcrossTenants is the core tenant-scoping guarantee: a
// broadcast for tenant A must NOT reach a subscriber of tenant B.
func TestEventsDoNotLeakAcrossTenants(t *testing.T) {
	h := NewHub()
	const tenantA, tenantB int64 = 1, 2
	chA, unsubA := h.Subscribe(tenantA)
	defer unsubA()
	chB, unsubB := h.Subscribe(tenantB)
	defer unsubB()

	h.Broadcast(Event{TenantID: tenantA, Entity: "invoice", ID: 7, Action: "create"})

	// A receives it.
	select {
	case e := <-chA:
		if e.ID != 7 {
			t.Fatalf("tenant A got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("tenant A did not receive its own event")
	}
	// B must NOT receive it.
	select {
	case e := <-chB:
		t.Fatalf("tenant B leaked event %+v", e)
	case <-time.After(150 * time.Millisecond):
		// expected: nothing delivered to B
	}
}

// TestGlobalEventReachesAllTenants verifies the catalogue (global) sentinel:
// an Event with TenantID == GlobalTenantID reaches every subscriber regardless
// of tenant.
func TestGlobalEventReachesAllTenants(t *testing.T) {
	h := NewHub()
	chA, unsubA := h.Subscribe(1)
	defer unsubA()
	chB, unsubB := h.Subscribe(2)
	defer unsubB()

	h.Broadcast(Event{TenantID: GlobalTenantID, Entity: "catalog_version", ID: 3, Action: "ingest"})

	for name, ch := range map[string]<-chan Event{"A": chA, "B": chB} {
		select {
		case e := <-ch:
			if e.Entity != "catalog_version" || e.ID != 3 {
				t.Fatalf("tenant %s got %+v", name, e)
			}
		case <-time.After(time.Second):
			t.Fatalf("tenant %s did not receive global event", name)
		}
	}
}
