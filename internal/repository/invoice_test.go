package repository

import (
	"context"
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
)

func TestInvoiceCreateNumbersAndTotals(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, InvoiceInput{
		ParticipantID: pid, IssueDate: "2026-01-01", DueDate: "2026-01-31", Tax: 10,
	}, []billing.LineItemInput{
		{Code: "01_011_0107_1_1", Description: "Support", Quantity: 2, UnitPrice: 50, GstFree: true},
		{Description: "Travel", Quantity: 1, UnitPrice: 5},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.Number != "INV-0001" {
		t.Fatalf("Number = %q, want INV-0001", inv.Number)
	}
	if inv.Subtotal != 105 || inv.Tax != 10 || inv.Total != 115 {
		t.Fatalf("totals = %.2f/%.2f/%.2f, want 105/10/115", inv.Subtotal, inv.Tax, inv.Total)
	}
	if len(inv.LineItems) != 2 || !inv.LineItems[0].GstFree || inv.LineItems[0].LineTotal != 100 {
		t.Fatalf("line items = %+v", inv.LineItems)
	}

	// Second invoice increments the per-tenant number.
	inv2, err := repo.Create(ctx, tid, InvoiceInput{ParticipantID: pid, IssueDate: "2026-02-01", DueDate: "2026-02-28"},
		[]billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 1}})
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if inv2.Number != "INV-0002" {
		t.Fatalf("Number 2 = %q, want INV-0002", inv2.Number)
	}
}

func TestInvoiceGetWithBalance(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, InvoiceInput{ParticipantID: pid, IssueDate: "2026-01-01", DueDate: "2026-01-31"},
		[]billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 100}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := NewPayments(conn).Create(ctx, tid, PaymentInput{InvoiceID: inv.ID, Amount: 30, PaidAt: "2026-01-05"}); err != nil {
		t.Fatalf("pay: %v", err)
	}
	got, err := repo.Get(ctx, tid, inv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TotalPaid != 30 || got.Balance != 70 {
		t.Fatalf("paid/balance = %v/%v, want 30/70", got.TotalPaid, got.Balance)
	}
	if got.ParticipantName != "Jane" {
		t.Fatalf("ParticipantName = %q, want Jane", got.ParticipantName)
	}
}

func TestInvoiceUpdateAndStatus(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, InvoiceInput{ParticipantID: pid, IssueDate: "2026-01-01", DueDate: "2026-01-31"},
		[]billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 100}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := repo.Update(ctx, tid, inv.ID, InvoiceInput{ParticipantID: pid, IssueDate: "2026-01-01", DueDate: "2026-02-15", Tax: 5},
		[]billing.LineItemInput{{Description: "Y", Quantity: 2, UnitPrice: 10}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if up.Number != "INV-0001" || up.Subtotal != 20 || up.Total != 25 || len(up.LineItems) != 1 {
		t.Fatalf("Update = %+v", up)
	}
	if err := repo.UpdateStatus(ctx, tid, inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, _ := repo.Get(ctx, tid, inv.ID)
	if got.Status != "sent" {
		t.Fatalf("Status = %q, want sent", got.Status)
	}
}

func TestInvoiceListAndDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	a, _ := repo.Create(ctx, tid, InvoiceInput{ParticipantID: pid, IssueDate: "2026-01-01", DueDate: "2026-01-31"},
		[]billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 1}})
	if list, err := repo.List(ctx, tid); err != nil || len(list) != 1 {
		t.Fatalf("List len=%d err=%v", len(list), err)
	}
	if err := repo.Delete(ctx, tid, a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, a.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestInvoiceParticipantStats(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	inv, _ := repo.Create(ctx, tid, InvoiceInput{ParticipantID: pid, IssueDate: "2026-01-01", DueDate: "2026-01-31"},
		[]billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 200}})
	if _, err := NewPayments(conn).Create(ctx, tid, PaymentInput{InvoiceID: inv.ID, Amount: 50, PaidAt: "2026-01-05"}); err != nil {
		t.Fatalf("pay: %v", err)
	}
	stats, err := repo.ParticipantStats(ctx, tid, pid)
	if err != nil {
		t.Fatalf("ParticipantStats: %v", err)
	}
	if stats.InvoiceCount != 1 || stats.TotalInvoiced != 200 || stats.TotalPaid != 50 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestInvoiceTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	pidA := seedParticipant(t, conn, a, "A Jane")
	repo := NewInvoices(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, a, InvoiceInput{ParticipantID: pidA, IssueDate: "2026-01-01", DueDate: "2026-01-31"},
		[]billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 100}})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Tenant B cannot read tenant A's invoice or see it in lists.
	if got, _ := repo.Get(ctx, b, inv.ID); got != nil {
		t.Fatalf("tenant B read tenant A's invoice: %+v", got)
	}
	if list, _ := repo.List(ctx, b); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
	// Per-tenant numbering: tenant B's first invoice is also INV-0001.
	pidB := seedParticipant(t, conn, b, "B Bob")
	invB, err := repo.Create(ctx, b, InvoiceInput{ParticipantID: pidB, IssueDate: "2026-01-01", DueDate: "2026-01-31"},
		[]billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 1}})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if invB.Number != "INV-0001" {
		t.Fatalf("tenant B first invoice number = %q, want INV-0001 (per-tenant)", invB.Number)
	}
}
