package app

import (
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newPaymentServer wires the payment routes behind RequireSession + ResolveTenant, plus participant
// and invoice creation so payments can reference a real invoice.
func newPaymentServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "payment.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)
	pH := participant.NewHandler(participant.NewService(conn, hub))
	invH := invoice.NewHandler(invoice.NewService(conn, conn, hub, shift.NewService(conn, conn, hub, invoice.NewInvoices(conn))))
	payH := invoice.NewPaymentHandler(invoice.NewPaymentService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Post("/participants", pH.Create)
			invH.Routes(pr)
			payH.Routes(pr)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// createPaymentInvoice posts a single-line invoice (unitPrice 25, qty 1, no tax →
// 25) and returns its id.
func createPaymentInvoice(t *testing.T, c *http.Client, base, uuid string, participantID string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"participantId": participantID, "issueDate": "2026-06-01", "dueDate": "2026-07-01",
		"lineItems": []map[string]any{
			{"description": "Work", "quantity": 1, "unitPrice": 25, "sortOrder": 0},
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

func TestPaymentRecordAndList(t *testing.T) {
	srv, uuid := newPaymentServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, uuid, "Acme")
	invID := createPaymentInvoice(t, c, srv.URL, uuid, participantID)

	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+invID+"/payments", `{"amount":10,"paymentDate":"2026-06-05"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("record: want 201 got %d", resp.StatusCode)
	}
	var p struct {
		ID     string  `json:"id"`
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode payment: %v", err)
	}
	if p.ID == "" || p.Amount != 10 {
		t.Fatalf("payment = %+v", p)
	}

	lr := get(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+invID+"/payments")
	defer func() { _ = lr.Body.Close() }()
	if lr.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", lr.StatusCode)
	}
	var list []struct {
		ID     string  `json:"id"`
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].Amount != 10 {
		t.Fatalf("list = %+v", list)
	}
}

func TestPaymentDeleteFlow(t *testing.T) {
	srv, uuid := newPaymentServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, uuid, "Acme")
	invID := createPaymentInvoice(t, c, srv.URL, uuid, participantID)

	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+invID+"/payments", `{"amount":10,"paymentDate":"2026-06-05"}`)
	var p struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode payment: %v", err)
	}
	_ = resp.Body.Close()

	dr := delete_(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+invID+"/payments/"+p.ID)
	_ = dr.Body.Close()
	if dr.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", dr.StatusCode)
	}
}

func TestPaymentDeleteMissing404(t *testing.T) {
	srv, uuid := newPaymentServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, uuid, "Acme")
	invID := createPaymentInvoice(t, c, srv.URL, uuid, participantID)
	dr := delete_(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+invID+"/payments/"+uuidpkg.NewString())
	defer func() { _ = dr.Body.Close() }()
	if dr.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing: want 404 got %d", dr.StatusCode)
	}
}

func TestPaymentZeroAmount400(t *testing.T) {
	srv, uuid := newPaymentServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, uuid, "Acme")
	invID := createPaymentInvoice(t, c, srv.URL, uuid, participantID)

	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices/"+invID+"/payments", `{"amount":0,"paymentDate":"2026-06-05"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("zero amount: want 400 got %d", resp.StatusCode)
	}
}

func TestPaymentRecordUnauthenticated401(t *testing.T) {
	srv, uuid := newPaymentServer(t)
	c := jarClient(t)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices/1/payments", `{"amount":10,"paymentDate":"2026-06-05"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon record: want 401 got %d", resp.StatusCode)
	}
}
