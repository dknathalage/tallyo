package app

import (
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/catalogue"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/session"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newEstimateServer wires the estimate routes behind RequireSession+ResolveTenant, plus
// client creation so estimates can reference a valid client FK, and the
// invoice list so converted invoices can be observed.
func newEstimateServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "estimate.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)
	estH := estimate.NewHandler(estimate.NewService(conn, hub))
	invH := invoice.NewHandler(invoice.NewService(conn, hub, session.NewService(conn, hub, invoice.NewInvoices(conn))))
	pH := client.NewHandler(client.NewService(conn, hub))
	catH := catalogue.NewHandler(catalogue.NewService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Post("/clients", pH.Create)
			catH.Routes(pr)
			invH.Routes(pr)
			estH.Routes(pr)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// createEstimate posts a two-line estimate for the given client and returns
// the id. Lines total 25. The J10 validation engine computes tax from the lines
// (no tenant default tax rate → tax 0), so the total equals the subtotal (25).
func createEstimate(t *testing.T, c *http.Client, base, uuid string, clientID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "validUntil": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create estimate: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode estimate: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("create estimate: want non-empty uuid got %q", out.ID)
	}
	return out.ID
}

func TestEstimateCreateComputesTotalsAndNumber(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")

	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "validUntil": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201 got %d", resp.StatusCode)
	}
	var est struct {
		Number    string  `json:"number"`
		Total     float64 `json:"total"`
		Subtotal  float64 `json:"subtotal"`
		LineItems []struct {
			Description string `json:"description"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&est); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if est.Number != "EST-0001" {
		t.Fatalf("number: want EST-0001 got %q", est.Number)
	}
	if est.Subtotal != 25 {
		t.Fatalf("subtotal: want 25 got %v", est.Subtotal)
	}
	if est.Total != 25 {
		t.Fatalf("total: want 25 got %v", est.Total)
	}
	if len(est.LineItems) != 2 {
		t.Fatalf("lineItems: want 2 got %d", len(est.LineItems))
	}
}

func TestEstimateGetReturnsLineItems(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createEstimate(t, c, srv.URL, uuid, clientID)

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
	var est struct {
		LineItems []struct {
			Description string `json:"description"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&est); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(est.LineItems) != 2 {
		t.Fatalf("lineItems: want 2 got %d", len(est.LineItems))
	}
}

func TestEstimateStatusFlip(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createEstimate(t, c, srv.URL, uuid, clientID)

	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/status", `{"status":"sent"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200 got %d", resp.StatusCode)
	}
}

func TestEstimateDuplicate(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createEstimate(t, c, srv.URL, uuid, clientID)

	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/duplicate", `{}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("duplicate: want 201 got %d", resp.StatusCode)
	}
	var est struct {
		Number string `json:"number"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&est); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if est.Number != "EST-0002" {
		t.Fatalf("duplicate number: want EST-0002 got %q", est.Number)
	}
}

func TestEstimateBulkStatusAndDelete(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	a := createEstimate(t, c, srv.URL, uuid, clientID)
	b := createEstimate(t, c, srv.URL, uuid, clientID)

	sr := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/bulk-status", `{"ids":["`+a+`","`+b+`"],"status":"sent"}`)
	_ = sr.Body.Close()
	if sr.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-status: want 204 got %d", sr.StatusCode)
	}

	dr := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/bulk-delete", `{"ids":["`+a+`","`+b+`"]}`)
	_ = dr.Body.Close()
	if dr.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", dr.StatusCode)
	}
}

func TestEstimateConvert(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createEstimate(t, c, srv.URL, uuid, clientID)

	// A draft estimate cannot be converted.
	draftResp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/convert", `{}`)
	_ = draftResp.Body.Close()
	if draftResp.StatusCode != http.StatusConflict {
		t.Fatalf("convert draft: want 409 got %d", draftResp.StatusCode)
	}

	// Accept it, then convert.
	sr := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/status", `{"status":"accepted"}`)
	_ = sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		t.Fatalf("accept: want 200 got %d", sr.StatusCode)
	}

	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/convert", `{}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("convert: want 200 got %d", resp.StatusCode)
	}
	var res struct {
		InvoiceUUID    string `json:"id"`
		InvoiceNumber  string `json:"invoiceNumber"`
		EstimateNumber string `json:"estimateNumber"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.InvoiceNumber == "" {
		t.Fatalf("convert: invoiceNumber empty")
	}
	if res.InvoiceUUID == "" {
		t.Fatalf("convert: want non-empty invoice uuid got %q", res.InvoiceUUID)
	}
	if res.EstimateNumber != "EST-0001" {
		t.Fatalf("convert: estimateNumber want EST-0001 got %q", res.EstimateNumber)
	}

	// The converted invoice is now present in the invoice list.
	lr := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices")
	defer func() { _ = lr.Body.Close() }()
	if lr.StatusCode != http.StatusOK {
		t.Fatalf("invoice list: want 200 got %d", lr.StatusCode)
	}
	var invs []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&invs); err != nil {
		t.Fatalf("decode invoice list: %v", err)
	}
	if len(invs) != 1 || invs[0].ID != res.InvoiceUUID {
		t.Fatalf("converted invoice not in list: %+v", invs)
	}

	// Converting again is a conflict.
	again := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/convert", `{}`)
	_ = again.Body.Close()
	if again.StatusCode != http.StatusConflict {
		t.Fatalf("convert again: want 409 got %d", again.StatusCode)
	}
}

// TestEstimateConvertTwiceConflicts verifies that converting an already-
// converted estimate returns 409 (ErrAlreadyConverted → conflict in handler).
func TestEstimateConvertTwiceConflicts(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createEstimate(t, c, srv.URL, uuid, clientID)

	// Accept the estimate so it is eligible for conversion.
	sr := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/status", `{"status":"accepted"}`)
	_ = sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		t.Fatalf("accept: want 200 got %d", sr.StatusCode)
	}

	// First convert succeeds.
	r1 := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/convert", `{}`)
	_ = r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		t.Fatalf("first convert: want 200 got %d", r1.StatusCode)
	}

	// Second convert must be 409 (ErrAlreadyConverted → handler writes 409).
	r2 := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/convert", `{}`)
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusConflict {
		t.Fatalf("second convert: want 409 got %d", r2.StatusCode)
	}
}

func TestEstimateCreateNoItems400(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "validUntil": "2026-02-01", "lineItems": []any{},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no items: want 400 got %d", resp.StatusCode)
	}
}

func TestEstimateGetMissing404(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+uuidpkg.NewString())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestEstimateConvertMissing404(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+uuidpkg.NewString()+"/convert", `{}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("convert missing: want 404 got %d", resp.StatusCode)
	}
}

func TestEstimateListUnauthenticated401(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/estimates")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}

func TestEstimatePdf(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createEstimate(t, c, srv.URL, uuid, clientID)

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+id+"/pdf")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pdf: want 200 got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type: want application/pdf got %q", ct)
	}
	b := make([]byte, 4)
	if _, err := resp.Body.Read(b); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if string(b) != "%PDF" {
		t.Fatalf("pdf header: want %%PDF got %q", string(b))
	}
}

func TestEstimatePdfMissing404(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+uuidpkg.NewString()+"/pdf")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing pdf: want 404 got %d", resp.StatusCode)
	}
}

// TestEstimateLineItemCustomItemRoundTrips verifies a custom-item uuid set on an
// estimate line item round-trips on GET.
func TestEstimateLineItemCustomItemRoundTrips(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	catalogueItemID := createCatalogueItem(t, c, srv.URL, uuid, "Mileage")

	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "validUntil": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "Trip", "quantity": 3, "unitPrice": 0.85, "sortOrder": 0, "catalogueItemId": catalogueItemID},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201 got %d", resp.StatusCode)
	}
	var est struct {
		ID        string `json:"id"`
		LineItems []struct {
			CatalogueItemID *string `json:"catalogueItemId"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&est); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(est.LineItems) != 1 || est.LineItems[0].CatalogueItemID == nil || *est.LineItems[0].CatalogueItemID != catalogueItemID {
		t.Fatalf("create customItemId: want %q got %v", catalogueItemID, est.LineItems)
	}

	getResp := get(t, c, srv.URL+"/api/t/"+uuid+"/estimates/"+est.ID)
	defer func() { _ = getResp.Body.Close() }()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", getResp.StatusCode)
	}
	var got struct {
		LineItems []struct {
			CatalogueItemID *string `json:"catalogueItemId"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if len(got.LineItems) != 1 || got.LineItems[0].CatalogueItemID == nil || *got.LineItems[0].CatalogueItemID != catalogueItemID {
		t.Fatalf("get customItemId: want %q got %v", catalogueItemID, got.LineItems)
	}
}

// TestEstimateLineItemUnknownCatalogueItem422 verifies an unknown catalogue-item
// uuid on an estimate line item is rejected by the line validator (422).
func TestEstimateLineItemUnknownCatalogueItem422(t *testing.T) {
	srv, uuid := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")

	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "validUntil": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "Trip", "quantity": 1, "unitPrice": 5, "sortOrder": 0, "catalogueItemId": uuidpkg.NewString()},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown catalogue item: want 422 got %d", resp.StatusCode)
	}
}
