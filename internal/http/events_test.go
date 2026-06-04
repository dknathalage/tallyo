package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
)

func TestEventsStreamsBroadcast(t *testing.T) {
	hub := realtime.NewHub()
	h := NewEventsHandler(hub)
	srv := httptest.NewServer(http.HandlerFunc(h.Stream))
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
	hub.Broadcast(realtime.Event{Entity: "invoice", ID: 7, Action: "update"})

	// read lines until we see a data: frame containing the event
	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.HasPrefix(line, "data:") && strings.Contains(line, `"invoice"`) && strings.Contains(line, `"id":7`) {
			return // success
		}
	}
	t.Fatal("did not receive expected SSE data frame")
}

func TestNewEventsHandlerNilHubPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil hub")
		}
	}()
	NewEventsHandler(nil)
}
