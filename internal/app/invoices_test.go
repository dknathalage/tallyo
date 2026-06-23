package app

import (
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/customitem"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/session"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newInvoiceServer wires the invoice routes behind RequireSession+ResolveTenant, plus client
// creation so invoices can reference a valid client FK.
func newInvoiceServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "invoice.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)
	invH := invoice.NewHandler(invoice.NewService(conn, conn, hub, session.NewService(conn, conn, hub, invoice.NewInvoices(conn))))
	pH := client.NewHandler(client.NewService(conn, hub))
	ciH := customitem.NewHandler(customitem.NewService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Post("/clients", pH.Create)
			ciH.Routes(pr)
			invH.Routes(pr)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// createInvoice posts a two-line invoice for the given client and returns
// the id. Lines total 2*10 + 1*5 = 25. The J10 validation engine now COMPUTES
// tax from the lines (these custom lines aren't GST-free but the tenant has no
// default tax rate → tax 0), so the client-supplied "tax" field is ignored and
// the total equals the subtotal (25).
func createInvoice(t *testing.T, c *http.Client, base, uuid string, clientID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create invoice: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode invoice: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("create invoice: want non-empty uuid got %q", out.ID)
	}
	return out.ID
}

func TestInvoiceCreateComputesTotalsAndSnapshots(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")

	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201 got %d", resp.StatusCode)
	}
	var inv struct {
		Number           string  `json:"number"`
		Total            float64 `json:"total"`
		Subtotal         float64 `json:"subtotal"`
		BusinessSnapshot string  `json:"businessSnapshot"`
		ClientSnap       string  `json:"clientSnapshot"`
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
	if inv.ClientSnap == "" || inv.BusinessSnapshot == "" {
		t.Fatalf("snapshots must be populated: business=%q client=%q", inv.BusinessSnapshot, inv.ClientSnap)
	}
}

func TestInvoiceGetReturnsLineItems(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createInvoice(t, c, srv.URL, uuid, clientID)

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+id)
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
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	_ = createInvoice(t, c, srv.URL, uuid, clientID)

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	var out []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("list: want 1 got %d", len(out))
	}
}

func TestInvoiceStatusFlip(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createInvoice(t, c, srv.URL, uuid, clientID)

	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+id+"/status", `{"status":"sent"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200 got %d", resp.StatusCode)
	}
}

func TestInvoiceBulkStatusAndDelete(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	a := createInvoice(t, c, srv.URL, uuid, clientID)
	b := createInvoice(t, c, srv.URL, uuid, clientID)

	sr := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices/bulk-status", `{"ids":["`+a+`","`+b+`"],"status":"sent"}`)
	_ = sr.Body.Close()
	if sr.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-status: want 204 got %d", sr.StatusCode)
	}

	dr := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices/bulk-delete", `{"ids":["`+a+`","`+b+`"]}`)
	_ = dr.Body.Close()
	if dr.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", dr.StatusCode)
	}
}

func TestInvoiceClientStats(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	_ = createInvoice(t, c, srv.URL, uuid, clientID)

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/clients/"+clientID+"/stats")
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
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "dueDate": "2026-02-01", "lineItems": []any{},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no items: want 400 got %d", resp.StatusCode)
	}
}

func TestInvoiceGetMissing404(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+uuidpkg.NewString())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestInvoiceListUnauthenticated401(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}

func TestInvoicePdf(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	id := createInvoice(t, c, srv.URL, uuid, clientID)

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+id+"/pdf")
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
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+uuidpkg.NewString()+"/pdf")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing pdf: want 404 got %d", resp.StatusCode)
	}
}

// TestInvoiceLineItemCustomItemRoundTrips verifies a line item created with a
// customItemId (a custom-item uuid) reports that same uuid on GET — the int FK
// never crosses the API.
func TestInvoiceLineItemCustomItemRoundTrips(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")
	customItemID := createCustomItem(t, c, srv.URL, uuid, "Mileage")

	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "Trip", "quantity": 3, "unitPrice": 0.85, "sortOrder": 0, "customItemId": customItemID},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: want 201 got %d", resp.StatusCode)
	}
	var inv struct {
		ID        string `json:"id"`
		LineItems []struct {
			CustomItemID *string `json:"customItemId"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(inv.LineItems) != 1 {
		t.Fatalf("line items: want 1 got %d", len(inv.LineItems))
	}
	if inv.LineItems[0].CustomItemID == nil || *inv.LineItems[0].CustomItemID != customItemID {
		t.Fatalf("create customItemId: want %q got %v", customItemID, inv.LineItems[0].CustomItemID)
	}

	getResp := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+inv.ID)
	defer func() { _ = getResp.Body.Close() }()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", getResp.StatusCode)
	}
	var got struct {
		LineItems []struct {
			CustomItemID *string `json:"customItemId"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if len(got.LineItems) != 1 || got.LineItems[0].CustomItemID == nil || *got.LineItems[0].CustomItemID != customItemID {
		t.Fatalf("get customItemId: want %q got %v", customItemID, got.LineItems)
	}
}

// TestInvoiceLineItemUnknownCustomItem400 verifies an unknown custom-item uuid
// on a line item is rejected at the write boundary.
func TestInvoiceLineItemUnknownCustomItem400(t *testing.T) {
	srv, uuid := newInvoiceServer(t)
	c := loggedInClient(t, srv.URL)
	clientID := createClient(t, c, srv.URL, uuid, "Acme")

	body, err := json.Marshal(map[string]any{
		"clientId": clientID, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			{"description": "Trip", "quantity": 1, "unitPrice": 5, "sortOrder": 0, "customItemId": uuidpkg.NewString()},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown custom item: want 400 got %d", resp.StatusCode)
	}
}
