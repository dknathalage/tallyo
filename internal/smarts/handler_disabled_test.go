package smarts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDisabledHandlerReturns503(t *testing.T) {
	h := NewHandler(nil, false)
	r := chi.NewRouter()
	h.Routes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, path := range []string{"/smarts/draft-invoice", "/smarts/suggest-lines", "/smarts/follow-up"} {
		resp, err := http.Post(srv.URL+path, "application/json", nil)
		if err != nil {
			t.Fatalf("post %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s: want 503 got %d", path, resp.StatusCode)
		}
	}
	// map-import is gated by RequireRole — without auth wiring it returns 401/403
	// before the guard, so it is intentionally excluded here.
}

func TestWriteSmartError(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{ErrNotFound, http.StatusNotFound},
		{ErrNoData, http.StatusUnprocessableEntity},
		{errStub{}, http.StatusBadGateway},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeSmartError(rec, c.err)
		if rec.Code != c.code {
			t.Fatalf("%v: want %d got %d", c.err, c.code, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "stub-internal") {
			t.Fatal("raw error string leaked into body")
		}
	}
}

type errStub struct{}

func (errStub) Error() string { return "stub-internal model failure" }
