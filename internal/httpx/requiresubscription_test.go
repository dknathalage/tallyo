package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/reqctx"
)

func TestRequireSubscription(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot) // 418 = "next ran"
	})
	h := RequireSubscription(next)

	cases := []struct {
		name     string
		method   string
		setFlag  bool // whether the entitled flag is present
		entitled bool
		want     int
	}{
		{"entitled write passes", http.MethodPost, true, true, http.StatusTeapot},
		{"lapsed read passes", http.MethodGet, true, false, http.StatusTeapot},
		{"lapsed POST blocked", http.MethodPost, true, false, http.StatusPaymentRequired},
		{"lapsed PUT blocked", http.MethodPut, true, false, http.StatusPaymentRequired},
		{"lapsed PATCH blocked", http.MethodPatch, true, false, http.StatusPaymentRequired},
		{"lapsed DELETE blocked", http.MethodDelete, true, false, http.StatusPaymentRequired},
		{"no flag (gate off) write passes", http.MethodPost, false, false, http.StatusTeapot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", nil)
			if tc.setFlag {
				req = req.WithContext(reqctx.WithEntitled(req.Context(), tc.entitled))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
