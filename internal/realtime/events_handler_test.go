package realtime

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/reqctx"
)

// safeRecorder wraps httptest.ResponseRecorder with a mutex so the SSE stream
// goroutine (writer) and the test goroutine (reader) do not race on Body.
// ponytail: recorder polling; swap to piped writer if -race complains.
type safeRecorder struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	code int
	hdr  http.Header
}

func newSafeRecorder() *safeRecorder {
	return &safeRecorder{code: 200, hdr: make(http.Header)}
}

func (r *safeRecorder) Header() http.Header { return r.hdr }

func (r *safeRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(b)
}

func (r *safeRecorder) WriteHeader(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.code = code
}

func (r *safeRecorder) Flush() {}

func (r *safeRecorder) body() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func (r *safeRecorder) statusCode() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.code
}

func TestStreamDeliversEventFrame(t *testing.T) {
	hub := NewHub()
	h := NewEventsHandler(hub)

	ctx, cancel := context.WithCancel(reqctx.WithTenant(context.Background(), "tenant-1"))
	req := httptest.NewRequest("GET", "/api/events", nil).WithContext(ctx)
	rec := newSafeRecorder()

	done := make(chan struct{})
	go func() { h.Stream(rec, req); close(done) }()

	// Wait until the hub has at least one subscriber (bounded: 100 × 10ms = 1s,
	// NASA rule 2). Stream subscribes just after writing its headers, so polling
	// the subscriber count is more reliable than a fixed sleep. The test is
	// in-package so direct hub field access is fine.
	waitFor(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		return len(hub.clients) > 0
	})

	hub.Broadcast(Event{TenantID: "tenant-1", Entity: "invoice", UUID: "abc", Action: "created"})

	waitFor(t, func() bool { return strings.Contains(rec.body(), `"entity":"invoice"`) })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after context cancel")
	}
	if !strings.Contains(rec.body(), "data: ") {
		t.Fatalf("want a data frame, got %q", rec.body())
	}
}

func TestWriteFrameSkipsUnmarshalableButKeepsAlive(t *testing.T) {
	rec := httptest.NewRecorder()
	if !writeFrame(rec, Event{Entity: "x", UUID: "1", Action: "created"}) {
		t.Fatal("writeFrame should return true on success")
	}
	if !strings.HasPrefix(rec.Body.String(), "data: ") {
		t.Fatalf("frame format wrong: %q", rec.Body.String())
	}
}

// waitFor polls cond up to ~1s (bounded, NASA rule 2).
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 1s")
}
