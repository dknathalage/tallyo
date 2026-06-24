// Package realtime provides an in-process Server-Sent-Events hub.
//
// After a mutation commits, the service layer calls Hub.Broadcast(Event);
// the SSE HTTP handler subscribes per connected client and streams events.
// The hub is concurrency-safe and never blocks the broadcaster on a slow
// client: each client has a bounded buffer, and on overflow the client's
// channel is closed so it reconnects and resyncs.
//
// # Tenant scoping
//
// Each subscriber registers WITH its tenant id (Subscribe(tenantID)). An event
// is delivered ONLY to subscribers of the same tenant, so one tenant never
// observes another tenant's changes (cross-tenant isolation, spec §8).
//
// The single EXCEPTION is a broadcast with no owning tenant in scope (e.g. a
// price-list import that runs outside a request tenant context). A broadcast
// carrying the sentinel TenantID == GlobalTenantID (0) is delivered to EVERY
// current subscriber regardless of their tenant, so such a change reaches all
// tenants' open streams.
package realtime

import "sync"

// clientBufferSize bounds each subscriber's channel. A slow client that
// fails to drain within this many pending events is dropped on overflow.
const clientBufferSize = 16

// GlobalTenantID is the sentinel tenant id for events that belong to no single
// tenant (e.g. a price-list import running outside a request tenant). A
// Broadcast whose Event.TenantID equals GlobalTenantID is delivered to every
// subscriber. Real tenant ids are non-empty uuid strings, so "" can never
// collide with a real tenant.
const GlobalTenantID string = ""

// Event is a change notification serialized to subscribers by the SSE handler.
// TenantID routes the event: a real tenant id (>=1) reaches only that tenant's
// subscribers; GlobalTenantID (0) reaches all subscribers. TenantID is NOT
// serialized to clients — it is a server-side routing field only.
//
// UUID is the changed entity's public identifier (a uuid string). The SPA uses
// it to know which entity changed and refetch. Int primary keys never cross the
// API (spec: "int PK never crosses the API"), so the payload carries the uuid,
// not the int PK. Bulk/sweep events that touch no single entity carry "".
type Event struct {
	TenantID string `json:"-"`
	Entity   string `json:"entity"`
	UUID     string `json:"id"`
	Action   string `json:"action"`
}

// client holds a single subscriber's delivery channel and the tenant it is
// scoped to. once guards close so the channel is never closed twice
// (unsubscribe and overflow both funnel through removeLocked).
type client struct {
	ch       chan Event
	tenantID string
	once     sync.Once
}

// Hub fans Events out to subscribers, routing each event to the subscribers of
// its tenant (plus, for global events, to all subscribers).
type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

// NewHub returns a ready-to-use Hub.
func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

// Subscribe registers a new subscriber scoped to tenantID and returns its
// read-only event channel plus an unsubscribe function. Calling unsubscribe
// more than once is safe. The channel is closed when the subscriber is removed
// (by unsubscribe or by overflow drop). The subscriber receives events
// broadcast for its own tenant plus global events (TenantID == GlobalTenantID).
func (h *Hub) Subscribe(tenantID string) (<-chan Event, func()) {
	c := &client{ch: make(chan Event, clientBufferSize), tenantID: tenantID}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.removeLocked(c)
	}
	return c.ch, unsub
}

// Broadcast delivers e to the subscribers it is routed to without blocking. A
// real tenant id reaches only that tenant's subscribers; GlobalTenantID reaches
// all subscribers. A subscriber whose buffer is full is dropped (its channel
// closed) so it can reconnect and resync. The loop is bounded by the number of
// subscribers.
func (h *Hub) Broadcast(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if e.TenantID != GlobalTenantID && c.tenantID != e.TenantID {
			continue // not this subscriber's tenant
		}
		select {
		case c.ch <- e:
		default:
			h.removeLocked(c)
		}
	}
}

// removeLocked deletes c and closes its channel exactly once. Callers must
// hold h.mu. Deleting during a range over the map is safe in Go.
func (h *Hub) removeLocked(c *client) {
	delete(h.clients, c)
	c.once.Do(func() { close(c.ch) })
}
