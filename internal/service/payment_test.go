package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/repository"
)

// newPaymentSvc wires a migrated DB with a payment service, the hub, and an
// invoices/clients repo so a seeded invoice can be paid against.
func newPaymentSvc(t *testing.T) (*PaymentService, *realtime.Hub, *repository.InvoicesRepo, *repository.ClientsRepo) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "pay.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	hub := realtime.NewHub()
	return NewPaymentService(conn, hub), hub, repository.NewInvoices(conn), repository.NewClients(conn)
}

// seedInvoice creates a client and a single-line invoice (rate 25, qty 1) so the
// total is a clean 25.
func seedInvoice(t *testing.T, invoices *repository.InvoicesRepo, clients *repository.ClientsRepo) *repository.Invoice {
	t.Helper()
	c, err := clients.Create(context.Background(), repository.ClientInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("seed client: %v", err)
	}
	inv, err := invoices.Create(context.Background(), repository.InvoiceInput{
		ClientID: c.ID, Date: "2026-06-01", DueDate: "2026-07-01",
	}, []repository.LineItemInput{{Description: "Work", Quantity: 1, Rate: 25}})
	if err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	return inv
}

func TestPaymentCreateBroadcastsPaymentAndInvoice(t *testing.T) {
	svc, hub, invoices, clients := newPaymentSvc(t)
	inv := seedInvoice(t, invoices, clients)
	ch, unsub := hub.Subscribe()
	defer unsub()
	ctx := context.Background()

	p, err := svc.Create(ctx, repository.PaymentInput{
		InvoiceID: inv.ID, Amount: 10, PaymentDate: "2026-06-05",
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
	svc, hub, invoices, clients := newPaymentSvc(t)
	inv := seedInvoice(t, invoices, clients)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if _, err := svc.Create(context.Background(), repository.PaymentInput{
		InvoiceID: inv.ID, Amount: 0, PaymentDate: "2026-06-05",
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
	svc, _, invoices, clients := newPaymentSvc(t)
	inv := seedInvoice(t, invoices, clients)
	ctx := context.Background()

	if _, err := svc.Create(ctx, repository.PaymentInput{InvoiceID: inv.ID, Amount: 10, PaymentDate: "2026-06-05"}); err != nil {
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
	svc, hub, invoices, clients := newPaymentSvc(t)
	inv := seedInvoice(t, invoices, clients)
	ctx := context.Background()

	p, err := svc.Create(ctx, repository.PaymentInput{InvoiceID: inv.ID, Amount: 10, PaymentDate: "2026-06-05"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, unsub := hub.Subscribe()
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
	svc, hub, _, _ := newPaymentSvc(t)
	ch, unsub := hub.Subscribe()
	defer unsub()

	if err := svc.Delete(context.Background(), 99999); err == nil {
		t.Fatal("delete missing must error")
	}
	select {
	case e := <-ch:
		t.Fatalf("no event expected on failed delete, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}
