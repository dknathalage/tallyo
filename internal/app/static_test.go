package app

import (
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func newTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": {Data: []byte("INDEX")},
		"200.html":   {Data: []byte("SPA_FALLBACK")},
		"_app/x.js":  {Data: []byte("JSDATA")},
	}
}

func TestSPAServesAsset(t *testing.T) {
	h := httpx.SPAHandler(newTestFS())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/_app/x.js", nil))
	if w.Code != 200 || w.Body.String() != "JSDATA" {
		t.Fatalf("asset: code=%d body=%q", w.Code, w.Body.String())
	}
}

func TestSPAFallsBackTo200(t *testing.T) {
	h := httpx.SPAHandler(newTestFS())
	for _, path := range []string{"/", "/settings", "/clients/42"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Body.String() != "SPA_FALLBACK" {
			t.Fatalf("path %s: body=%q want SPA_FALLBACK", path, w.Body.String())
		}
	}
}
