package invoice

import (
	"testing"
)

// newPaymentSvc wires a migrated DB with a payment service, an invoices repo, and
// a seeded tenant + client so a seeded invoice can be paid.
func newPaymentSvc(t *testing.T) (*PaymentService, *InvoicesRepo, string, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	return NewPaymentService(conn), NewInvoices(conn), tenantID, clientID
}

func TestPaymentCreate(t *testing.T) {
	svc, invoices, tenantID, clientID := newPaymentSvc(t)
	inv := seedInvoiceSvc(t, invoices, tenantID, clientID)
	ctx := tctx(tenantID)

	p, err := svc.Create(ctx, PaymentInput{
		InvoiceID: inv.ID, Amount: 10, PaidAt: "2026-06-05",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p == nil || p.ID == "" {
		t.Fatalf("Create returned %+v", p)
	}
}

func TestPaymentCreateZeroAmountErrors(t *testing.T) {
	svc, invoices, tenantID, clientID := newPaymentSvc(t)
	inv := seedInvoiceSvc(t, invoices, tenantID, clientID)

	if _, err := svc.Create(tctx(tenantID), PaymentInput{
		InvoiceID: inv.ID, Amount: 0, PaidAt: "2026-06-05",
	}); err == nil {
		t.Fatal("zero amount must error")
	}
}

func TestPaymentListForInvoice(t *testing.T) {
	svc, invoices, tenantID, clientID := newPaymentSvc(t)
	inv := seedInvoiceSvc(t, invoices, tenantID, clientID)
	ctx := tctx(tenantID)

	if _, err := svc.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: 10, PaidAt: "2026-06-05"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	list, err := svc.ListForInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("ListForInvoice: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d, want 1", len(list))
	}
}

func TestPaymentDelete(t *testing.T) {
	svc, invoices, tenantID, clientID := newPaymentSvc(t)
	inv := seedInvoiceSvc(t, invoices, tenantID, clientID)
	ctx := tctx(tenantID)

	p, err := svc.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: 10, PaidAt: "2026-06-05"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list, err := svc.ListForInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("ListForInvoice: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete = %d, want 0", len(list))
	}
}

func TestPaymentDeleteMissingReturnsErr(t *testing.T) {
	svc, _, tenantID, _ := newPaymentSvc(t)

	if err := svc.Delete(tctx(tenantID), "nonexistent-uuid"); err == nil {
		t.Fatal("delete missing must error")
	}
}
