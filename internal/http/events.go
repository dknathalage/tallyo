package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
)

// EventsHandler serves the Server-Sent-Events stream of change events.
type EventsHandler struct {
	hub *realtime.Hub
}

// NewEventsHandler builds an EventsHandler. It panics on a nil hub since the
// handler cannot function without one (programmer error at wiring time).
func NewEventsHandler(hub *realtime.Hub) *EventsHandler {
	if hub == nil {
		panic("NewEventsHandler: nil hub")
	}
	return &EventsHandler{hub: hub}
}

// Stream is the SSE endpoint. Auth is enforced by upstream middleware. It
// subscribes to the hub, writes data frames per event, sends periodic
// heartbeats, and returns (cleaning up its subscription) when the client
// disconnects or the hub drops the subscriber on overflow.
func (h *EventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsub := h.hub.Subscribe()
	defer unsub()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case e, ok := <-ch:
			if !ok {
				return // hub closed our channel (overflow) → client reconnects
			}
			if !writeFrame(w, e) {
				return // client gone or unmarshalable event already skipped
			}
			flusher.Flush()
		}
	}
}

// writeFrame marshals e and writes a single SSE data frame. It returns false
// only when a write fails (client gone). A marshal error skips the event but
// keeps the stream alive (returns true).
func writeFrame(w http.ResponseWriter, e realtime.Event) bool {
	data, err := json.Marshal(e)
	if err != nil {
		return true // skip a bad event rather than kill the stream
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return false
	}
	if _, err := w.Write(data); err != nil {
		return false
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return false
	}
	return true
}
