// Package realtime provides an in-process Server-Sent-Events hub.
//
// After a mutation commits, the service layer calls Hub.Broadcast(Event);
// the SSE HTTP handler subscribes per connected client and streams events.
// The hub is concurrency-safe and never blocks the broadcaster on a slow
// client: each client has a bounded buffer, and on overflow the client's
// channel is closed so it reconnects and resyncs.
package realtime

import "sync"

// clientBufferSize bounds each subscriber's channel. A slow client that
// fails to drain within this many pending events is dropped on overflow.
const clientBufferSize = 16

// Event is a change notification serialized to subscribers by the SSE handler.
type Event struct {
	Entity string `json:"entity"`
	ID     int64  `json:"id"`
	Action string `json:"action"`
}

// client holds a single subscriber's delivery channel. once guards close so
// the channel is never closed twice (unsubscribe and overflow both funnel
// through removeLocked).
type client struct {
	ch   chan Event
	once sync.Once
}

// Hub fans Events out to all current subscribers.
type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

// NewHub returns a ready-to-use Hub.
func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

// Subscribe registers a new subscriber and returns its read-only event
// channel plus an unsubscribe function. Calling unsubscribe more than once
// is safe. The channel is closed when the subscriber is removed (by
// unsubscribe or by overflow drop).
func (h *Hub) Subscribe() (<-chan Event, func()) {
	c := &client{ch: make(chan Event, clientBufferSize)}
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

// Broadcast delivers e to every current subscriber without blocking. A
// subscriber whose buffer is full is dropped (its channel closed) so it can
// reconnect and resync. The loop is bounded by the number of subscribers.
func (h *Hub) Broadcast(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
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
