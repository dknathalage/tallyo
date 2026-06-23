package payer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// newPMHandler builds a handler over a fresh DB and returns it with the tenant
// id and a seeded payer.
func newPMHandler(t *testing.T) (*Handler, int64, *Payer) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	svc := NewService(conn, realtime.NewHub())
	seeded, err := svc.Create(tctx(tenantID), PayerInput{Name: "Acme PM", Email: "a@b.com"})
	if err != nil {
		t.Fatalf("seed payer: %v", err)
	}
	return NewHandler(svc), tenantID, seeded
}

// mountPM returns a router with the slice routes mounted and a middleware that
// attaches the tenant id to every request context (standing in for auth).
func mountPM(h *Handler, tenantID int64) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)
	return r
}

func TestPayerGetByUUID(t *testing.T) {
	h, tenantID, seeded := newPMHandler(t)
	srv := httptest.NewServer(mountPM(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/payers/" + seeded.UUID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["id"] != seeded.UUID {
		t.Fatalf("json id=%v want uuid %q", got["id"], seeded.UUID)
	}
}

func TestPayerGetUnknownUUID404(t *testing.T) {
	h, tenantID, _ := newPMHandler(t)
	srv := httptest.NewServer(mountPM(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/payers/3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestPayerGetNonUUID400(t *testing.T) {
	h, tenantID, _ := newPMHandler(t)
	srv := httptest.NewServer(mountPM(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/payers/123")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}
