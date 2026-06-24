package realtime

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

const testTenant string = "t-1"

func TestSubscribeReceivesBroadcast(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe(testTenant)
	defer unsub()

	h.Broadcast(Event{TenantID: testTenant, Entity: "invoice", UUID: "u1", Action: "update"})

	select {
	case e := <-ch:
		if e.Entity != "invoice" || e.UUID != "u1" || e.Action != "update" {
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
	h.Broadcast(Event{TenantID: testTenant, Entity: "x", UUID: "u1", Action: "update"})
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
			h.Broadcast(Event{TenantID: testTenant, Entity: "e", UUID: "u", Action: "update"})
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
		go func(tid string) {
			defer wg.Done()
			ch, unsub := h.Subscribe(tid)
			defer unsub()
			for j := 0; j < 50; j++ {
				h.Broadcast(Event{TenantID: tid, Entity: "e", UUID: "u", Action: "update"})
				select {
				case <-ch:
				default:
				}
			}
		}(fmt.Sprintf("t-%d", i%3+1))
	}
	wg.Wait()
}

// TestEventsDoNotLeakAcrossTenants is the core tenant-scoping guarantee: a
// broadcast for tenant A must NOT reach a subscriber of tenant B.
func TestEventsDoNotLeakAcrossTenants(t *testing.T) {
	h := NewHub()
	const tenantA, tenantB string = "t-1", "t-2"
	chA, unsubA := h.Subscribe(tenantA)
	defer unsubA()
	chB, unsubB := h.Subscribe(tenantB)
	defer unsubB()

	h.Broadcast(Event{TenantID: tenantA, Entity: "invoice", UUID: "u7", Action: "create"})

	// A receives it.
	select {
	case e := <-chA:
		if e.UUID != "u7" {
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
	chA, unsubA := h.Subscribe("t-1")
	defer unsubA()
	chB, unsubB := h.Subscribe("t-2")
	defer unsubB()

	h.Broadcast(Event{TenantID: GlobalTenantID, Entity: "catalog_version", UUID: "cv3", Action: "ingest"})

	for name, ch := range map[string]<-chan Event{"A": chA, "B": chB} {
		select {
		case e := <-ch:
			if e.Entity != "catalog_version" || e.UUID != "cv3" {
				t.Fatalf("tenant %s got %+v", name, e)
			}
		case <-time.After(time.Second):
			t.Fatalf("tenant %s did not receive global event", name)
		}
	}
}

// TestEventJSONIDIsUUIDString is the Task 2.8 guarantee: the serialized SSE
// payload's "id" is the entity uuid (a string), never an int PK. TenantID stays
// server-side only ("-").
func TestEventJSONIDIsUUIDString(t *testing.T) {
	e := Event{TenantID: "t-5", Entity: "invoice", UUID: "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c", Action: "update"}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	id, ok := generic["id"]
	if !ok {
		t.Fatalf("payload has no id field: %s", raw)
	}
	if _, isString := id.(string); !isString {
		t.Fatalf("id is not a string (leaked int?): %T %v in %s", id, id, raw)
	}
	if id != "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c" {
		t.Fatalf("id = %v, want the uuid", id)
	}
	if _, leaked := generic["tenantId"]; leaked {
		t.Fatalf("tenantId leaked into payload: %s", raw)
	}
}
