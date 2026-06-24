package app

import (
	"bufio"
	"context"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// testTenant is the tenant the SSE test streams subscribe under (the real
// handler reads it from reqctx, attached upstream by httpx.RequireAuth).
const testTenant string = "t-1"

// withTenant injects a tenant into the request context, standing in for the
// httpx.RequireAuth middleware that runs before Stream in production.
func withTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(reqctx.WithTenant(r.Context(), testTenant)))
	})
}

func TestEventsStreamsBroadcast(t *testing.T) {
	hub := realtime.NewHub()
	h := realtime.NewEventsHandler(hub)
	srv := httptest.NewServer(withTenant(http.HandlerFunc(h.Stream)))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type=%q", ct)
	}

	// give the handler a moment to subscribe, then broadcast
	time.Sleep(50 * time.Millisecond)
	const invoiceUUID = "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c"
	hub.Broadcast(realtime.Event{TenantID: testTenant, Entity: "invoice", UUID: invoiceUUID, Action: "update"})

	// read lines until we see a data: frame containing the event. The payload "id"
	// is the entity uuid (a string), never an int PK (spec: int PK never crosses
	// the API).
	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.HasPrefix(line, "data:") && strings.Contains(line, `"invoice"`) && strings.Contains(line, `"id":"`+invoiceUUID+`"`) {
			return // success
		}
	}
	t.Fatal("did not receive expected SSE data frame")
}

// TestEventsStreamsThroughMiddleware guards the realtime path through the real
// wrapper chain (httpx.RequestLogger's statusWriter + scs sessionResponseWriter).
// A plain w.(http.Flusher) assertion would fail or silently buffer here; the
// handler must flush via http.ResponseController, which unwraps both wrappers.
func TestEventsStreamsThroughMiddleware(t *testing.T) {
	hub := realtime.NewHub()
	h := realtime.NewEventsHandler(hub)
	sm := scs.New()
	handler := httpx.RequestLogger(sm.LoadAndSave(withTenant(http.HandlerFunc(h.Stream))))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}

	time.Sleep(50 * time.Millisecond)
	hub.Broadcast(realtime.Event{TenantID: testTenant, Entity: "business_profile", UUID: "", Action: "update"})

	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.HasPrefix(line, "data:") && strings.Contains(line, `"business_profile"`) {
			return // success: frame flushed through the middleware chain
		}
	}
	t.Fatal("no SSE data frame delivered through middleware chain")
}

func TestNewEventsHandlerNilHubPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil hub")
		}
	}()
	realtime.NewEventsHandler(nil)
}
