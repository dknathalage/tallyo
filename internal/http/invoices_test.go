package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newInvoiceServer wires the invoice routes behind RequireAuth, plus client and
// tax-rate creation so invoices can reference valid FKs.
func newInvoiceServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "invoice.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	users := auth.NewUsers(conn)
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := users.Create(t.Context(), "o@x.com", hash, "owner"); err != nil {
		t.Fatalf("Create owner: %v", err)
	}

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	invH := NewInvoiceHandler(service.NewInvoiceService(conn, hub))
	cH := NewClientHandler(service.NewClientService(conn, hub))
	trH := NewTaxRateHandler(service.NewTaxRateService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Post("/clients", cH.Create)
			pr.Post("/tax-rates", trH.Create)
			pr.Get("/invoices", invH.List)
			pr.Post("/invoices", invH.Create)
			pr.Post("/invoices/bulk-delete", invH.BulkDelete)
			pr.Post("/invoices/bulk-status", invH.BulkStatus)
			pr.Get("/invoices/{id}", invH.Get)
			pr.Put("/invoices/{id}", invH.Update)
			pr.Delete("/invoices/{id}", invH.Delete)
			pr.Post("/invoices/{id}/status", invH.Status)
			pr.Post("/invoices/{id}/duplicate", invH.Duplicate)
			pr.Get("/invoices/{id}/pdf", invH.Pdf)
			pr.Get("/clients/{id}/stats", invH.ClientStats)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createInvoice posts a two-line invoice for the given client and returns the id.
func createInvoice(t *testing.T, c *http.Client, base string, clientID int64) int64 {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "date": "2026-01-01", "dueDate": "2026-02-01", "taxRate": 10,
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "rate": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "rate": 5, "sortOrder": 1},
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
	clientID := createClient(t, c, srv.URL, "Acme")

	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "date": "2026-01-01", "dueDate": "2026-02-01", "taxRate": 10,
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "rate": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "rate": 5, "sortOrder": 1},
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
		InvoiceNumber    string  `json:"invoiceNumber"`
		Total            float64 `json:"total"`
		Subtotal         float64 `json:"subtotal"`
		BusinessSnapshot string  `json:"businessSnapshot"`
		ClientSnapshot   string  `json:"clientSnapshot"`
		LineItems        []struct {
			Description string  `json:"description"`
			Amount      float64 `json:"amount"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inv.InvoiceNumber != "INV-0001" {
		t.Fatalf("invoiceNumber: want INV-0001 got %q", inv.InvoiceNumber)
	}
	if inv.Subtotal != 25 {
		t.Fatalf("subtotal: want 25 got %v", inv.Subtotal)
	}
	if inv.Total != 27.5 {
		t.Fatalf("total: want 27.5 got %v", inv.Total)
	}
	if len(inv.LineItems) != 2 {
		t.Fatalf("lineItems: want 2 got %d", len(inv.LineItems))
	}
	if inv.ClientSnapshot == "" || inv.BusinessSnapshot == "" {
		t.Fatalf("snapshots must be populated: business=%q client=%q", inv.BusinessSnapshot, inv.ClientSnapshot)
	}
}

func TestInvoiceGetReturnsLineItems(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, "Acme")
	id := createInvoice(t, c, srv.URL, clientID)

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
	clientID := createClient(t, c, srv.URL, "Acme")
	_ = createInvoice(t, c, srv.URL, clientID)

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
	clientID := createClient(t, c, srv.URL, "Acme")
	id := createInvoice(t, c, srv.URL, clientID)

	resp := postJSON(t, c, srv.URL+"/api/invoices/"+itoa(id)+"/status", `{"status":"sent"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200 got %d", resp.StatusCode)
	}
}

func TestInvoiceDuplicate(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, "Acme")
	id := createInvoice(t, c, srv.URL, clientID)

	resp := postJSON(t, c, srv.URL+"/api/invoices/"+itoa(id)+"/duplicate", `{}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("duplicate: want 201 got %d", resp.StatusCode)
	}
	var inv struct {
		InvoiceNumber string `json:"invoiceNumber"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inv.InvoiceNumber != "INV-0002" {
		t.Fatalf("duplicate number: want INV-0002 got %q", inv.InvoiceNumber)
	}
}

func TestInvoiceBulkStatusAndDelete(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, "Acme")
	a := createInvoice(t, c, srv.URL, clientID)
	b := createInvoice(t, c, srv.URL, clientID)

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

func TestInvoiceClientStats(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, "Acme")
	_ = createInvoice(t, c, srv.URL, clientID)

	resp := get(t, c, srv.URL+"/api/clients/"+itoa(clientID)+"/stats")
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
	if stats.TotalInvoiced != 27.5 {
		t.Fatalf("totalInvoiced: want 27.5 got %v", stats.TotalInvoiced)
	}
}

func TestInvoiceCreateNoItems400(t *testing.T) {
	srv := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, "Acme")
	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "date": "2026-01-01", "dueDate": "2026-02-01", "lineItems": []any{},
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
	clientID := createClient(t, c, srv.URL, "Acme")
	id := createInvoice(t, c, srv.URL, clientID)

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
