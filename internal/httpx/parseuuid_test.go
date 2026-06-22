package httpx

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestParseUUID(t *testing.T) {
	cases := []struct {
		name   string
		param  string
		want   string
		wantOK bool
	}{
		{"valid", "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c", "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c", true},
		{"empty", "", "", false},
		{"not-a-uuid", "123", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("uuid", tc.param)
			r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
			got, ok := ParseUUID(r, "uuid")
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("ParseUUID=%q,%v want %q,%v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
