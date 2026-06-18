package service

import (
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// newPaymentSvc wires a migrated DB with a payment service, the hub, an invoices
// repo, and a seeded tenant + participant so a seeded invoice can be paid.
func newPaymentSvc(t *testing.T) (*PaymentService, *realtime.Hub, *repository.InvoicesRepo, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn)
	participantID := seedParticipant(t, conn, tenantID)
	hub := realtime.NewHub()
	return NewPaymentService(conn, hub), hub, repository.NewInvoices(conn), tenantID, participantID
}

// seedInvoice creates a single-line invoice (unit price 25, qty 1) so the total
// is a clean 25.
func seedInvoice(t *testing.T, invoices *repository.InvoicesRepo, tenantID, participantID int64) *repository.Invoice {
	t.Helper()
	inv, err := invoices.Create(tctx(tenantID), tenantID, repository.InvoiceInput{
		ParticipantID: participantID, IssueDate: "2026-06-01", DueDate: "2026-07-01",
	}, []billing.LineItemInput{{Description: "Work", Quantity: 1, UnitPrice: 25}})
	if err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	return inv
}

func TestPaymentCreateBroadcastsPaymentAndInvoice(t *testing.T) {
	svc, hub, invoices, tenantID, participantID := newPaymentSvc(t)
	inv := seedInvoice(t, invoices, tenantID, participantID)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()
	ctx := tctx(tenantID)

	p, err := svc.Create(ctx, repository.PaymentInput{
		InvoiceID: inv.ID, Amount: 10, PaidAt: "2026-06-05",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p == nil || p.ID <= 0 {
		t.Fatalf("Create returned %+v", p)
	}

	gotPayment := false
	gotInvoice := false
	for i := 0; i < 2; i++ { // exactly two events expected
		select {
		case e := <-ch:
			if e.Entity == "payment" && e.ID == p.ID && e.Action == "create" {
				gotPayment = true
			} else if e.Entity == "invoice" && e.ID == inv.ID && e.Action == "update" {
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
	svc, hub, invoices, tenantID, participantID := newPaymentSvc(t)
	inv := seedInvoice(t, invoices, tenantID, participantID)
	ch, unsub := hub.Subscribe(tenantID)
	defer unsub()

	if _, err := svc.Create(tctx(tenantID), repository.PaymentInput{
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
	svc, _, invoices, tenantID, participantID := newPaymentSvc(t)
	inv := seedInvoice(t, invoices, tenantID, participantID)
	ctx := tctx(tenantID)

	if _, err := svc.Create(ctx, repository.PaymentInput{InvoiceID: inv.ID, Amount: 10, PaidAt: "2026-06-05"}); err != nil {
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
	svc, hub, invoices, tenantID, participantID := newPaymentSvc(t)
	inv := seedInvoice(t, invoices, tenantID, participantID)
	ctx := tctx(tenantID)

	p, err := svc.Create(ctx, repository.PaymentInput{InvoiceID: inv.ID, Amount: 10, PaidAt: "2026-06-05"})
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
			if e.Entity == "payment" && e.ID == p.ID && e.Action == "delete" {
				gotPayment = true
			} else if e.Entity == "invoice" && e.ID == inv.ID && e.Action == "update" {
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

	if err := svc.Delete(tctx(tenantID), 99999); err == nil {
		t.Fatal("delete missing must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed delete, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
