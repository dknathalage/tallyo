package invoice

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/realtime"
)

// newPaymentSvc wires a migrated DB with a payment service, the hub, an invoices
// repo, and a seeded tenant + client so a seeded invoice can be paid.
func newPaymentSvc(t *testing.T) (*PaymentService, *realtime.Hub, *InvoicesRepo, string, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	hub := realtime.NewHub()
	return NewPaymentService(conn, hub), hub, NewInvoices(conn), tenantID, clientID
}

func TestPaymentCreateBroadcastsPaymentAndInvoice(t *testing.T) {
	svc, hub, invoices, tenantID, clientID := newPaymentSvc(t)
	inv := seedInvoiceSvc(t, invoices, tenantID, clientID)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
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

	gotPayment := false
	gotInvoice := false
	for i := 0; i < 2; i++ { // exactly two events expected
		select {
		case e := <-ch:
			if e.Entity == "payment" && e.UUID == p.ID && e.Action == "create" {
				gotPayment = true
			} else if e.Entity == "invoice" && e.UUID == inv.ID && e.Action == "update" {
				gotInvoice = true
			} else {
				t.Fatalf("unexpected event %+v", e)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing broadcast after Create (payment=%v invoice=%v)", gotPayment, gotInvoice)
		}
	}
	if !gotPayment || !gotInvoice {
		t.Fatalf("expected payment+invoice events, got payment=%v invoice=%v", gotPayment, gotInvoice)
	}
}

func TestPaymentCreateZeroAmountNoEvent(t *testing.T) {
	svc, hub, invoices, tenantID, clientID := newPaymentSvc(t)
	inv := seedInvoiceSvc(t, invoices, tenantID, clientID)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), PaymentInput{
		InvoiceID: inv.ID, Amount: 0, PaidAt: "2026-06-05",
	}); err == nil {
		t.Fatal("zero amount must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed create, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

func TestPaymentListForInvoice(t *testing.T) {
	svc, _, invoices, tenantID, clientID := newPaymentSvc(t)
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

func TestPaymentDeleteBroadcastsPaymentAndInvoice(t *testing.T) {
	svc, hub, invoices, tenantID, clientID := newPaymentSvc(t)
	inv := seedInvoiceSvc(t, invoices, tenantID, clientID)
	ctx := tctx(tenantID)

	p, err := svc.Create(ctx, PaymentInput{InvoiceID: inv.ID, Amount: 10, PaidAt: "2026-06-05"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	gotPayment := false
	gotInvoice := false
	for i := 0; i < 2; i++ { // exactly two events expected
		select {
		case e := <-ch:
			if e.Entity == "payment" && e.UUID == p.ID && e.Action == "delete" {
				gotPayment = true
			} else if e.Entity == "invoice" && e.UUID == inv.ID && e.Action == "update" {
				gotInvoice = true
			} else {
				t.Fatalf("unexpected event %+v", e)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing broadcast after Delete (payment=%v invoice=%v)", gotPayment, gotInvoice)
		}
	}
	if !gotPayment || !gotInvoice {
		t.Fatalf("expected payment+invoice events, got payment=%v invoice=%v", gotPayment, gotInvoice)
	}
}

func TestPaymentDeleteMissingReturnsErr(t *testing.T) {
	svc, hub, _, tenantID, _ := newPaymentSvc(t)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if err := svc.Delete(tctx(tenantID), "nonexistent-uuid"); err == nil {
		t.Fatal("delete missing must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed delete, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
