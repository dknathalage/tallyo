package httpapi

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func newServerFS() fstest.MapFS {
	return fstest.MapFS{
		"200.html": {Data: []byte("SPA_FALLBACK")},
	}
}

func TestServerHealthz(t *testing.T) {
	s := NewServer(Deps{Assets: newServerFS()})
	w := httptest.NewRecorder()
	s.Router.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 || w.Body.String() != "ok" {
		t.Fatalf("healthz: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestServerSPAFallback(t *testing.T) {
	s := NewServer(Deps{Assets: newServerFS()})
	w := httptest.NewRecorder()
	s.Router.ServeHTTP(w, httptest.NewRequest("GET", "/someroute", nil))
	if w.Body.String() != "SPA_FALLBACK" {
		t.Fatalf("spa fallback: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestNewServerPanicsWithoutAssets(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when Assets is nil")
		}
	}()
	NewServer(Deps{})
}
