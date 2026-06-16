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
// The single EXCEPTION is the global NDIS Support Catalogue (spec §4.3), which
// is shared national reference data with no owning tenant. A broadcast carrying
// the sentinel TenantID == GlobalTenantID (0) is delivered to EVERY current
// subscriber regardless of their tenant, so a platform-admin catalogue ingest
// reaches all tenants' open streams.
package realtime

import "sync"

// clientBufferSize bounds each subscriber's channel. A slow client that
// fails to drain within this many pending events is dropped on overflow.
const clientBufferSize = 16

// GlobalTenantID is the sentinel tenant id for events that belong to no single
// tenant (the shared NDIS Support Catalogue). A Broadcast whose Event.TenantID
// equals GlobalTenantID is delivered to every subscriber. Real tenant ids are
// always >= 1 (AUTOINCREMENT), so 0 can never collide with a real tenant.
const GlobalTenantID int64 = 0

// Event is a change notification serialized to subscribers by the SSE handler.
// TenantID routes the event: a real tenant id (>=1) reaches only that tenant's
// subscribers; GlobalTenantID (0) reaches all subscribers. TenantID is NOT
// serialized to clients — it is a server-side routing field only.
type Event struct {
	TenantID int64  `json:"-"`
	Entity   string `json:"entity"`
	ID       int64  `json:"id"`
	Action   string `json:"action"`
}

// client holds a single subscriber's delivery channel and the tenant it is
// scoped to. once guards close so the channel is never closed twice
// (unsubscribe and overflow both funnel through removeLocked).
type client struct {
	ch       chan Event
	tenantID int64
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
func (h *Hub) Subscribe(tenantID int64) (<-chan Event, func()) {
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
