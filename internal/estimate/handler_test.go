package estimate

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// mountEstimate mounts the estimate routes on a fresh router with a one-line
// middleware injecting the tenant (stands in for auth).
func mountEstimate(h *Handler, tenantID int64) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	h.Routes(r)
	return r
}

// newEstimateHandler builds a fresh DB, seeds a tenant + client + a single
// estimate, and returns the handler, tenant id, client uuid, and estimate.
func newEstimateHandler(t *testing.T) (*Handler, int64, string, *Estimate) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	p, err := client.NewClients(conn).Create(tctx(tenantID), tenantID, client.ClientInput{Name: "Jane"})
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	hub := realtime.NewHub()
	svc := NewService(conn, hub)
	est := makeEstimate(t, svc, tenantID, p.ID)
	return NewHandler(svc), tenantID, p.UUID, est
}

func TestEstimateGetByUUID(t *testing.T) {
	h, tenantID, pUUID, est := newEstimateHandler(t)
	srv := httptest.NewServer(mountEstimate(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/estimates/" + est.UUID)
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
	if got["id"] != est.UUID {
		t.Fatalf("json id=%v want estimate uuid %q", got["id"], est.UUID)
	}
	if got["clientId"] != pUUID {
		t.Fatalf("json clientId=%v want client uuid %q", got["clientId"], pUUID)
	}
	lines, ok := got["lineItems"].([]any)
	if !ok || len(lines) == 0 {
		t.Fatalf("lineItems missing/empty: %v", got["lineItems"])
	}
	line0 := lines[0].(map[string]any)
	if _, err := uuid.Parse(line0["id"].(string)); err != nil {
		t.Fatalf("lineItems[0].id=%v not a uuid", line0["id"])
	}
}

func TestEstimateGetUnknownUUID404(t *testing.T) {
	h, tenantID, _, _ := newEstimateHandler(t)
	srv := httptest.NewServer(mountEstimate(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/estimates/" + ids.New())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestEstimateGetNonUUID400(t *testing.T) {
	h, tenantID, _, _ := newEstimateHandler(t)
	srv := httptest.NewServer(mountEstimate(h, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/estimates/123")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}

func TestEstimateDuplicateByUUID(t *testing.T) {
	h, tenantID, _, est := newEstimateHandler(t)
	srv := httptest.NewServer(mountEstimate(h, tenantID))
	defer srv.Close()

	res, err := http.Post(srv.URL+"/estimates/"+est.UUID+"/duplicate", "application/json", nil)
	if err != nil {
		t.Fatalf("POST duplicate: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("duplicate status=%d want 201", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	dupID, ok := got["id"].(string)
	if !ok {
		t.Fatalf("duplicate id not a string: %v", got["id"])
	}
	if _, err := uuid.Parse(dupID); err != nil {
		t.Fatalf("duplicate id=%v not a uuid", dupID)
	}
	if dupID == est.UUID {
		t.Fatalf("duplicate reused source uuid %q", est.UUID)
	}
}

func TestEstimateConvertReturnsInvoiceUUID(t *testing.T) {
	h, tenantID, _, est := newEstimateHandler(t)
	srv := httptest.NewServer(mountEstimate(h, tenantID))
	defer srv.Close()

	// Estimates can only be converted from 'accepted'. Flip the status first.
	body := []byte(`{"status":"accepted"}`)
	sres, err := http.Post(srv.URL+"/estimates/"+est.UUID+"/status", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST status: %v", err)
	}
	sres.Body.Close()
	if sres.StatusCode != http.StatusOK {
		t.Fatalf("status update=%d want 200", sres.StatusCode)
	}

	res, err := http.Post(srv.URL+"/estimates/"+est.UUID+"/convert", "application/json", nil)
	if err != nil {
		t.Fatalf("POST convert: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("convert status=%d want 200", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	invID, ok := got["id"].(string)
	if !ok {
		t.Fatalf("convert result id not a string: %v", got["id"])
	}
	if _, err := uuid.Parse(invID); err != nil {
		t.Fatalf("convert result id=%v not an invoice uuid", invID)
	}
}
