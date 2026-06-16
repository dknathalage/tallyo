package agent

import "sync"

// eventBufferSize bounds each subscriber's channel. A slow subscriber that
// fails to drain within this many pending events has the overflow event
// dropped (skipped) so a stalled consumer never blocks the agent loop.
const eventBufferSize = 32

// Event is one agent activity notification (plan, tool_result, access_request,
// message_final, error, ...) delivered to the conversation's subscribers.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// eventSub is one subscriber's delivery channel scoped to a conversation. once
// guards close so the channel is never closed twice (unsubscribe is idempotent).
type eventSub struct {
	ch   chan Event
	conv int64
	once sync.Once
}

// Events is a per-conversation pub/sub hub. It mirrors realtime.Hub semantics:
// a mutex guards the subscriber set and a full subscriber buffer drops the
// event (non-blocking send) so a slow client never stalls the publisher.
type Events struct {
	mu   sync.Mutex
	subs map[*eventSub]struct{}
}

// NewEvents returns a ready-to-use event hub.
func NewEvents() *Events {
	return &Events{subs: make(map[*eventSub]struct{})}
}

// Subscribe registers a subscriber scoped to convID and returns its read-only
// channel plus an unsubscribe func. Unsubscribe is safe to call more than once;
// it closes the channel exactly once.
func (e *Events) Subscribe(convID int64) (<-chan Event, func()) {
	if e == nil {
		panic("agent: Subscribe on nil Events")
	}
	s := &eventSub{ch: make(chan Event, eventBufferSize), conv: convID}
	e.mu.Lock()
	e.subs[s] = struct{}{}
	e.mu.Unlock()
	unsub := func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.removeLocked(s)
	}
	return s.ch, unsub
}

// Publish delivers ev to every subscriber of convID without blocking. A
// subscriber whose buffer is full has this event skipped (dropped). The loop is
// bounded by the number of subscribers.
func (e *Events) Publish(convID int64, ev Event) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for s := range e.subs { // bounded by len(e.subs)
		if s.conv != convID {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			// Buffer full: skip this event for this subscriber rather than block.
		}
	}
}

// removeLocked deletes s and closes its channel exactly once. Callers hold e.mu.
func (e *Events) removeLocked(s *eventSub) {
	if _, ok := e.subs[s]; !ok {
		return
	}
	delete(e.subs, s)
	s.once.Do(func() { close(s.ch) })
}
