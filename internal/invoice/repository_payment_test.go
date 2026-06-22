package invoice

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestPaymentCreateAndTotals(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	invID := seedInvoiceRepo(t, conn, tid, pid, 100)
	repo := NewPayments(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, tid, PaymentInput{InvoiceID: invID, Amount: 40, PaidAt: "2026-01-05", Method: "bank"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == 0 || p.Amount != 40 || p.Method != "bank" {
		t.Fatalf("Create = %+v", p)
	}
	if _, err := repo.Create(ctx, tid, PaymentInput{InvoiceID: invID, Amount: 25, PaidAt: "2026-01-06"}); err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	total, err := repo.TotalPaid(ctx, tid, invID)
	if err != nil || total != 65 {
		t.Fatalf("TotalPaid = %v err=%v, want 65", total, err)
	}
	list, err := repo.ListForInvoice(ctx, tid, invID)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListForInvoice len=%d err=%v", len(list), err)
	}
}

func TestPaymentRejectsBadInput(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPayments(conn)
	ctx := context.Background()
	if _, err := repo.Create(ctx, tid, PaymentInput{InvoiceID: 0, Amount: 1}); err == nil {
		t.Fatal("missing invoice: want error")
	}
	if _, err := repo.Create(ctx, tid, PaymentInput{InvoiceID: 1, Amount: 0}); err == nil {
		t.Fatal("non-positive amount: want error")
	}
}

func TestPaymentDeleteReturnsInvoiceID(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	invID := seedInvoiceRepo(t, conn, tid, pid, 100)
	repo := NewPayments(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, tid, PaymentInput{InvoiceID: invID, Amount: 10, PaidAt: "2026-01-05"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	gotUUID, gotInv, err := repo.Delete(ctx, tid, p.ID)
	if err != nil || gotInv != invID || gotUUID != p.UUID {
		t.Fatalf("Delete = (%q,%d) err=%v, want (%q,%d)", gotUUID, gotInv, err, p.UUID, invID)
	}
	if _, _, err := repo.Delete(ctx, tid, 99999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Delete missing err = %v, want sql.ErrNoRows", err)
	}
}

func TestPaymentTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	pid := seedParticipant(t, conn, a, "Jane")
	invID := seedInvoiceRepo(t, conn, a, pid, 100)
	repo := NewPayments(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, a, PaymentInput{InvoiceID: invID, Amount: 50, PaidAt: "2026-01-05"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Tenant B sees neither the payments nor the total.
	if list, _ := repo.ListForInvoice(ctx, b, invID); len(list) != 0 {
		t.Fatalf("tenant B saw tenant A's payments: %d", len(list))
	}
	if total, _ := repo.TotalPaid(ctx, b, invID); total != 0 {
		t.Fatalf("tenant B TotalPaid = %v, want 0", total)
	}
	// Tenant B cannot delete tenant A's payment.
	if _, _, err := repo.Delete(ctx, b, p.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("tenant B Delete A's payment err = %v, want sql.ErrNoRows", err)
	}
}
