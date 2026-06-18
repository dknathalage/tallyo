package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/go-chi/chi/v5"
)

// newInvoiceServer wires the invoice routes behind RequireAuth, plus participant
// creation so invoices can reference a valid participant FK.
func newInvoiceServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "invoice.db")
	users, _, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, auth.NewTenants(conn))
	invH := invoice.NewHandler(invoice.NewService(conn, hub, shift.NewShifts(conn)))
	pH := participant.NewHandler(participant.NewService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users, auth.NewTenants(conn)))
			pr.Post("/participants", pH.Create)
			pr.Get("/invoices", invH.List)
			pr.Post("/invoices", invH.Create)
			pr.Post("/invoices/bulk-delete", invH.BulkDelete)
			pr.Post("/invoices/bulk-status", invH.BulkStatus)
			pr.Get("/invoices/{id}", invH.Get)
			pr.Put("/invoices/{id}", invH.Update)
			pr.Delete("/invoices/{id}", invH.Delete)
			pr.Post("/invoices/{id}/status", invH.Status)
			pr.Get("/invoices/{id}/pdf", invH.Pdf)
			pr.Get("/participants/{id}/stats", invH.ParticipantStats)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createInvoice posts a two-line invoice for the given participant and returns
// the id. Lines total 2*10 + 1*5 = 25. The J10 validation engine now COMPUTES
// tax from the lines (these custom lines aren't GST-free but the tenant has no
// default tax rate → tax 0), so the client-supplied "tax" field is ignored and
// the total equals the subtotal (25).
func createInvoice(t *testing.T, c *http.Client, base string, participantID int64) int64 {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"participantId": participantID, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create invoice: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode invoice: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create invoice: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestInvoiceCreateComputesTotalsAndSnapshots(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")

	body, err := json.Marshal(map[string]any{
		"participantId": participantID, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201 got %d", resp.StatusCode)
	}
	var inv struct {
		Number           string  `json:"number"`
		Total            float64 `json:"total"`
		Subtotal         float64 `json:"subtotal"`
		BusinessSnapshot string  `json:"businessSnapshot"`
		ParticipantSnap  string  `json:"participantSnapshot"`
		LineItems        []struct {
			Description string  `json:"description"`
			LineTotal   float64 `json:"lineTotal"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inv.Number != "INV-0001" {
		t.Fatalf("number: want INV-0001 got %q", inv.Number)
	}
	if inv.Subtotal != 25 {
		t.Fatalf("subtotal: want 25 got %v", inv.Subtotal)
	}
	if inv.Total != 25 {
		t.Fatalf("total: want 25 got %v", inv.Total)
	}
	if len(inv.LineItems) != 2 {
		t.Fatalf("lineItems: want 2 got %d", len(inv.LineItems))
	}
	if inv.ParticipantSnap == "" || inv.BusinessSnapshot == "" {
		t.Fatalf("snapshots must be populated: business=%q participant=%q", inv.BusinessSnapshot, inv.ParticipantSnap)
	}
}

func TestInvoiceGetReturnsLineItems(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createInvoice(t, c, srv.URL, participantID)

	resp := get(t, c, srv.URL+"/api/invoices/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
	var inv struct {
		LineItems []struct {
			Description string `json:"description"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(inv.LineItems) != 2 {
		t.Fatalf("lineItems: want 2 got %d", len(inv.LineItems))
	}
}

func TestInvoiceListReturnsArray(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	_ = createInvoice(t, c, srv.URL, participantID)

	resp := get(t, c, srv.URL+"/api/invoices")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	var out []struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("list: want 1 got %d", len(out))
	}
}

func TestInvoiceStatusFlip(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createInvoice(t, c, srv.URL, participantID)

	resp := postJSON(t, c, srv.URL+"/api/invoices/"+itoa(id)+"/status", `{"status":"sent"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200 got %d", resp.StatusCode)
	}
}

func TestInvoiceBulkStatusAndDelete(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	a := createInvoice(t, c, srv.URL, participantID)
	b := createInvoice(t, c, srv.URL, participantID)

	sr := postJSON(t, c, srv.URL+"/api/invoices/bulk-status", `{"ids":[`+itoa(a)+`,`+itoa(b)+`],"status":"sent"}`)
	_ = sr.Body.Close()
	if sr.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-status: want 204 got %d", sr.StatusCode)
	}

	dr := postJSON(t, c, srv.URL+"/api/invoices/bulk-delete", `{"ids":[`+itoa(a)+`,`+itoa(b)+`]}`)
	_ = dr.Body.Close()
	if dr.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", dr.StatusCode)
	}
}

func TestInvoiceParticipantStats(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	_ = createInvoice(t, c, srv.URL, participantID)

	resp := get(t, c, srv.URL+"/api/participants/"+itoa(participantID)+"/stats")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stats: want 200 got %d", resp.StatusCode)
	}
	var stats struct {
		InvoiceCount  int64   `json:"invoiceCount"`
		TotalInvoiced float64 `json:"totalInvoiced"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.InvoiceCount != 1 {
		t.Fatalf("invoiceCount: want 1 got %d", stats.InvoiceCount)
	}
	if stats.TotalInvoiced != 25 {
		t.Fatalf("totalInvoiced: want 25 got %v", stats.TotalInvoiced)
	}
}

func TestInvoiceCreateNoItems400(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	body, err := json.Marshal(map[string]any{
		"participantId": participantID, "issueDate": "2026-01-01", "dueDate": "2026-02-01", "lineItems": []any{},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no items: want 400 got %d", resp.StatusCode)
	}
}

func TestInvoiceGetMissing404(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/invoices/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestInvoiceListUnauthenticated401(t *testing.T) {
	srv := newInvoiceServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/invoices")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}

func TestInvoicePdf(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createInvoice(t, c, srv.URL, participantID)

	resp := get(t, c, srv.URL+"/api/invoices/"+itoa(id)+"/pdf")
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

func TestInvoicePdfMissing404(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/invoices/99999/pdf")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing pdf: want 404 got %d", resp.StatusCode)
	}
}
