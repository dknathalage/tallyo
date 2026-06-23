package invoice

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// mountInvoice mounts the invoice + payment routes on a fresh router with a
// one-line middleware injecting the tenant (stands in for auth).
func mountInvoice(inv *Handler, pay *PaymentHandler, tenantID int64) chi.Router {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(reqctx.WithTenant(req.Context(), tenantID)))
		})
	})
	inv.Routes(r)
	pay.Routes(r)
	return r
}

// newInvoiceHandler builds a fresh DB, seeds a tenant + client + a single
// invoice, and returns the handlers, tenant id, client uuid, and invoice.
func newInvoiceHandler(t *testing.T) (*Handler, *PaymentHandler, int64, string, *Invoice) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	pid, pUUID := seedClientUUID(t, conn, tenantID, "Jane")
	hub := realtime.NewHub()
	svc := NewService(conn, conn, hub, shift.NewService(conn, conn, hub, NewInvoices(conn)))
	inv := makeInvoice(t, svc, tenantID, pid)
	return NewHandler(svc), NewPaymentHandler(NewPaymentService(conn, hub)), tenantID, pUUID, inv
}

func TestInvoiceGetByUUID(t *testing.T) {
	ih, ph, tenantID, pUUID, inv := newInvoiceHandler(t)
	srv := httptest.NewServer(mountInvoice(ih, ph, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/invoices/" + inv.UUID)
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
	if got["id"] != inv.UUID {
		t.Fatalf("json id=%v want invoice uuid %q", got["id"], inv.UUID)
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

func TestInvoiceGetUnknownUUID404(t *testing.T) {
	ih, ph, tenantID, _, _ := newInvoiceHandler(t)
	srv := httptest.NewServer(mountInvoice(ih, ph, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/invoices/" + uuid.NewString())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", res.StatusCode)
	}
}

func TestInvoiceGetNonUUID400(t *testing.T) {
	ih, ph, tenantID, _, _ := newInvoiceHandler(t)
	srv := httptest.NewServer(mountInvoice(ih, ph, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/invoices/123")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", res.StatusCode)
	}
}

func TestInvoicePaymentLifecycleByUUID(t *testing.T) {
	ih, ph, tenantID, _, inv := newInvoiceHandler(t)
	srv := httptest.NewServer(mountInvoice(ih, ph, tenantID))
	defer srv.Close()

	// POST a payment under the invoice uuid.
	body, _ := json.Marshal(map[string]any{"amount": 5, "paymentDate": "2026-01-05"})
	res, err := http.Post(srv.URL+"/invoices/"+inv.UUID+"/payments", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST payment: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST payment status=%d want 201", res.StatusCode)
	}
	var p map[string]any
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatalf("decode payment: %v", err)
	}
	payUUID, ok := p["id"].(string)
	if !ok {
		t.Fatalf("payment id not a string: %v", p["id"])
	}
	if _, err := uuid.Parse(payUUID); err != nil {
		t.Fatalf("payment id=%v not a uuid", payUUID)
	}

	// GET the payment list under the invoice uuid.
	lres, err := http.Get(srv.URL + "/invoices/" + inv.UUID + "/payments")
	if err != nil {
		t.Fatalf("GET payments: %v", err)
	}
	defer lres.Body.Close()
	if lres.StatusCode != http.StatusOK {
		t.Fatalf("GET payments status=%d want 200", lres.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(lres.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0]["id"] != payUUID {
		t.Fatalf("payment list=%v want one id %q", list, payUUID)
	}

	// DELETE by payment uuid under the invoice uuid.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/invoices/"+inv.UUID+"/payments/"+payUUID, nil)
	dres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE payment: %v", err)
	}
	defer dres.Body.Close()
	if dres.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE payment status=%d want 204", dres.StatusCode)
	}
}

func TestInvoiceDraftFromShiftsByUUID(t *testing.T) {
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	pid, _ := seedClientUUID(t, conn, tenantID, "Jane")
	hub := realtime.NewHub()
	shiftSvc := shift.NewService(conn, conn, hub, NewInvoices(conn))
	invSvc := NewService(conn, conn, hub, shiftSvc)
	ih := NewHandler(invSvc)
	ph := NewPaymentHandler(NewPaymentService(conn, hub))

	// Seed one recorded shift with one item, capture its uuid.
	sh, err := shiftSvc.Create(tctx(tenantID), shift.ShiftInput{ClientID: pid, ServiceDate: "2026-01-15", Status: "recorded"})
	if err != nil {
		t.Fatalf("seed shift: %v", err)
	}
	if _, err := gen.New(conn).CreateLineItem(context.Background(), gen.CreateLineItemParams{
		Uuid: uuid.NewString(), TenantID: tenantID, ShiftID: sql.NullInt64{Int64: sh.ID, Valid: true},
		Description: "Work", Quantity: 1, UnitPrice: 10, LineTotal: 10,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	srv := httptest.NewServer(mountInvoice(ih, ph, tenantID))
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"shiftIds": []string{sh.UUID}})
	res, err := http.Post(srv.URL+"/invoices/draft-from-shifts", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST draft-from-shifts: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("draft-from-shifts status=%d want 201", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := uuid.Parse(got["id"].(string)); err != nil {
		t.Fatalf("draft invoice id=%v not a uuid", got["id"])
	}
}

func TestInvoiceClientFilterByUUID(t *testing.T) {
	ih, ph, tenantID, pUUID, inv := newInvoiceHandler(t)
	srv := httptest.NewServer(mountInvoice(ih, ph, tenantID))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/invoices?client=" + pUUID)
	if err != nil {
		t.Fatalf("GET ?client: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 || list[0]["id"] != inv.UUID {
		t.Fatalf("client filter list=%v want one id %q", list, inv.UUID)
	}
}
