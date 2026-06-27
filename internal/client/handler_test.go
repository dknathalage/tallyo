package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/payer"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// newClientHandler builds a handler over a fresh DB and returns it with the
// tenant id, a seeded payer (its uuid), and a client seeded WITH that
// payer.
func newClientHandler(t *testing.T) (*Handler, string, string, *Client) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	pm, err := payer.NewPayers(conn).Create(tctx(tenantID), tenantID, payer.PayerInput{Name: "PM Co"})
	if err != nil {
		t.Fatalf("seed payer: %v", err)
	}
	svc := NewService(conn)
	seeded, err := svc.Create(tctx(tenantID), ClientInput{Name: "Jane", PayerUUID: &pm.ID})
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	return NewHandler(svc), tenantID, pm.ID, seeded
}

// mountClient returns a router with the slice routes mounted and a
// middleware that attaches the tenant id to every request (standing in for auth).
func mountClient(h *Handler, tenantID string) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)
	return r
}

func TestClientGetByUUID(t *testing.T) {
	h, tenantID, pmUUID, seeded := newClientHandler(t)
	srv := httptest.NewServer(mountClient(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/clients/" + seeded.ID)
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
	if got["id"] != seeded.ID {
		t.Fatalf("json id=%v want uuid %q", got["id"], seeded.ID)
	}
	if got["payerId"] != pmUUID {
		t.Fatalf("json payerId=%v want payer uuid %q", got["payerId"], pmUUID)
	}
}

func TestClientGetUnknownUUID404(t *testing.T) {
	h, tenantID, _, _ := newClientHandler(t)
	srv := httptest.NewServer(mountClient(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/clients/3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestClientGetNonUUID400(t *testing.T) {
	h, tenantID, _, _ := newClientHandler(t)
	srv := httptest.NewServer(mountClient(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/clients/123")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}

// TestClientCreateResolvesPayerUUID proves an inbound payerId
// uuid resolves to the FK and round-trips back as the same uuid; an unknown
// payer uuid is rejected with 400.
func TestClientCreateResolvesPayerUUID(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	pm, err := payer.NewPayers(conn).Create(tctx(tenantID), tenantID, payer.PayerInput{Name: "PM Co"})
	if err != nil {
		t.Fatalf("seed payer: %v", err)
	}
	h := NewHandler(NewService(conn))
	srv := httptest.NewServer(mountClient(h, tenantID))
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"name": "Jane", "payerId": pm.ID})
	res, err := http.Post(srv.URL+"/clients", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d want 201", res.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created["payerId"] != pm.ID {
		t.Fatalf("created payerId=%v want %q", created["payerId"], pm.ID)
	}

	// Update to clear the payer (empty string → NULL FK).
	createdUUID, _ := created["id"].(string)
	upBody, _ := json.Marshal(map[string]any{"name": "Jane", "payerId": nil})
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/clients/"+createdUUID, bytes.NewReader(upBody))
	req.Header.Set("Content-Type", "application/json")
	upRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer upRes.Body.Close()
	if upRes.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d want 200", upRes.StatusCode)
	}
	var updated map[string]any
	if err := json.NewDecoder(upRes.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated["payerId"] != nil {
		t.Fatalf("updated payerId=%v want nil", updated["payerId"])
	}

	// An unknown payer uuid is rejected with 400.
	badBody, _ := json.Marshal(map[string]any{"name": "Bob", "payerId": "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c"})
	badRes, err := http.Post(srv.URL+"/clients", "application/json", bytes.NewReader(badBody))
	if err != nil {
		t.Fatalf("POST bad: %v", err)
	}
	defer badRes.Body.Close()
	if badRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown payer uuid status=%d want 400", badRes.StatusCode)
	}
}
