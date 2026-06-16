package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newEstimateServer wires the estimate routes behind RequireAuth, plus
// participant creation so estimates can reference a valid participant FK, and the
// invoice list so converted invoices can be observed.
func newEstimateServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "estimate.db")
	users, _, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	estH := NewEstimateHandler(service.NewEstimateService(conn, hub))
	invH := NewInvoiceHandler(service.NewInvoiceService(conn, hub))
	pH := NewParticipantHandler(service.NewParticipantService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Post("/participants", pH.Create)
			pr.Get("/invoices", invH.List)
			pr.Get("/estimates", estH.List)
			pr.Post("/estimates", estH.Create)
			pr.Post("/estimates/bulk-delete", estH.BulkDelete)
			pr.Post("/estimates/bulk-status", estH.BulkStatus)
			pr.Get("/estimates/{id}", estH.Get)
			pr.Put("/estimates/{id}", estH.Update)
			pr.Delete("/estimates/{id}", estH.Delete)
			pr.Post("/estimates/{id}/status", estH.Status)
			pr.Post("/estimates/{id}/duplicate", estH.Duplicate)
			pr.Get("/estimates/{id}/pdf", estH.Pdf)
			pr.Post("/estimates/{id}/convert", estH.Convert)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createEstimate posts a two-line estimate for the given participant and returns
// the id. Lines total 25; with tax 2.5 the total is 27.5.
func createEstimate(t *testing.T, c *http.Client, base string, participantID int64) int64 {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"participantId": participantID, "issueDate": "2026-01-01", "validUntil": "2026-02-01", "tax": 2.5,
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create estimate: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode estimate: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create estimate: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestEstimateCreateComputesTotalsAndNumber(t *testing.T) {
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")

	body, err := json.Marshal(map[string]any{
		"participantId": participantID, "issueDate": "2026-01-01", "validUntil": "2026-02-01", "tax": 2.5,
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/estimates", string(body))
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
	if est.Total != 27.5 {
		t.Fatalf("total: want 27.5 got %v", est.Total)
	}
	if len(est.LineItems) != 2 {
		t.Fatalf("lineItems: want 2 got %d", len(est.LineItems))
	}
}

func TestEstimateGetReturnsLineItems(t *testing.T) {
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createEstimate(t, c, srv.URL, participantID)

	resp := get(t, c, srv.URL+"/api/estimates/"+itoa(id))
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
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createEstimate(t, c, srv.URL, participantID)

	resp := postJSON(t, c, srv.URL+"/api/estimates/"+itoa(id)+"/status", `{"status":"sent"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200 got %d", resp.StatusCode)
	}
}

func TestEstimateDuplicate(t *testing.T) {
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createEstimate(t, c, srv.URL, participantID)

	resp := postJSON(t, c, srv.URL+"/api/estimates/"+itoa(id)+"/duplicate", `{}`)
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
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	a := createEstimate(t, c, srv.URL, participantID)
	b := createEstimate(t, c, srv.URL, participantID)

	sr := postJSON(t, c, srv.URL+"/api/estimates/bulk-status", `{"ids":[`+itoa(a)+`,`+itoa(b)+`],"status":"sent"}`)
	_ = sr.Body.Close()
	if sr.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-status: want 204 got %d", sr.StatusCode)
	}

	dr := postJSON(t, c, srv.URL+"/api/estimates/bulk-delete", `{"ids":[`+itoa(a)+`,`+itoa(b)+`]}`)
	_ = dr.Body.Close()
	if dr.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", dr.StatusCode)
	}
}

func TestEstimateConvert(t *testing.T) {
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createEstimate(t, c, srv.URL, participantID)

	// A draft estimate cannot be converted.
	draftResp := postJSON(t, c, srv.URL+"/api/estimates/"+itoa(id)+"/convert", `{}`)
	_ = draftResp.Body.Close()
	if draftResp.StatusCode != http.StatusConflict {
		t.Fatalf("convert draft: want 409 got %d", draftResp.StatusCode)
	}

	// Accept it, then convert.
	sr := postJSON(t, c, srv.URL+"/api/estimates/"+itoa(id)+"/status", `{"status":"accepted"}`)
	_ = sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		t.Fatalf("accept: want 200 got %d", sr.StatusCode)
	}

	resp := postJSON(t, c, srv.URL+"/api/estimates/"+itoa(id)+"/convert", `{}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("convert: want 200 got %d", resp.StatusCode)
	}
	var res struct {
		InvoiceID      int64  `json:"invoiceId"`
		InvoiceNumber  string `json:"invoiceNumber"`
		EstimateNumber string `json:"estimateNumber"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.InvoiceNumber == "" {
		t.Fatalf("convert: invoiceNumber empty")
	}
	if res.InvoiceID <= 0 {
		t.Fatalf("convert: want invoiceId>0 got %d", res.InvoiceID)
	}
	if res.EstimateNumber != "EST-0001" {
		t.Fatalf("convert: estimateNumber want EST-0001 got %q", res.EstimateNumber)
	}

	// The converted invoice is now present in the invoice list.
	lr := get(t, c, srv.URL+"/api/invoices")
	defer func() { _ = lr.Body.Close() }()
	if lr.StatusCode != http.StatusOK {
		t.Fatalf("invoice list: want 200 got %d", lr.StatusCode)
	}
	var invs []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&invs); err != nil {
		t.Fatalf("decode invoice list: %v", err)
	}
	if len(invs) != 1 || invs[0].ID != res.InvoiceID {
		t.Fatalf("converted invoice not in list: %+v", invs)
	}

	// Converting again is a conflict.
	again := postJSON(t, c, srv.URL+"/api/estimates/"+itoa(id)+"/convert", `{}`)
	_ = again.Body.Close()
	if again.StatusCode != http.StatusConflict {
		t.Fatalf("convert again: want 409 got %d", again.StatusCode)
	}
}

func TestEstimateCreateNoItems400(t *testing.T) {
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	body, err := json.Marshal(map[string]any{
		"participantId": participantID, "issueDate": "2026-01-01", "validUntil": "2026-02-01", "lineItems": []any{},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no items: want 400 got %d", resp.StatusCode)
	}
}

func TestEstimateGetMissing404(t *testing.T) {
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/estimates/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestEstimateConvertMissing404(t *testing.T) {
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/estimates/99999/convert", `{}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("convert missing: want 404 got %d", resp.StatusCode)
	}
}

func TestEstimateListUnauthenticated401(t *testing.T) {
	srv := newEstimateServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/estimates")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}

func TestEstimatePdf(t *testing.T) {
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createEstimate(t, c, srv.URL, participantID)

	resp := get(t, c, srv.URL+"/api/estimates/"+itoa(id)+"/pdf")
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
	srv := newEstimateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/estimates/99999/pdf")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing pdf: want 404 got %d", resp.StatusCode)
	}
}
