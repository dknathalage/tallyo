package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/dknathalage/tallyo/internal/billing"
)

func mkEstimate(t *testing.T, repo *EstimatesRepo, tid, pid int64) *Estimate {
	t.Helper()
	est, err := repo.Create(context.Background(), tid, EstimateInput{
		ParticipantID: pid, IssueDate: "2026-01-01", ValidUntil: "2026-02-01", Tax: 10,
	}, []billing.LineItemInput{{Description: "Support", Quantity: 2, UnitPrice: 50}})
	if err != nil {
		t.Fatalf("Create estimate: %v", err)
	}
	return est
}

func TestEstimateCreateNumbersAndTotals(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewEstimates(conn)

	est := mkEstimate(t, repo, tid, pid)
	if est.Number != "EST-0001" {
		t.Fatalf("Number = %q, want EST-0001", est.Number)
	}
	if est.Subtotal != 100 || est.Tax != 10 || est.Total != 110 {
		t.Fatalf("totals = %.2f/%.2f/%.2f, want 100/10/110", est.Subtotal, est.Tax, est.Total)
	}
	if est.ParticipantName != "Jane" {
		t.Fatalf("ParticipantName = %q, want Jane", est.ParticipantName)
	}
}

func TestEstimateUpdateStatusAndConvert(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewEstimates(conn)
	ctx := context.Background()

	est := mkEstimate(t, repo, tid, pid)

	// Cannot convert until accepted.
	if _, err := repo.Convert(ctx, tid, est.ID); !errors.Is(err, ErrNotAccepted) {
		t.Fatalf("Convert before accept err = %v, want ErrNotAccepted", err)
	}
	if err := repo.UpdateStatus(ctx, tid, est.ID, "accepted"); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	res, err := repo.Convert(ctx, tid, est.ID)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.InvoiceNumber != "INV-0001" || res.EstimateNumber != "EST-0001" {
		t.Fatalf("Convert result = %+v", res)
	}
	// Estimate is now converted; a second convert is rejected.
	if _, err := repo.Convert(ctx, tid, est.ID); !errors.Is(err, ErrAlreadyConverted) {
		t.Fatalf("second Convert err = %v, want ErrAlreadyConverted", err)
	}
	// The produced invoice exists with the copied line.
	inv, err := NewInvoices(conn).Get(ctx, tid, res.InvoiceID)
	if err != nil || inv == nil || len(inv.LineItems) != 1 || inv.Total != 110 {
		t.Fatalf("converted invoice = %+v err=%v", inv, err)
	}
}

func TestEstimateDuplicate(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewEstimates(conn)
	ctx := context.Background()

	src := mkEstimate(t, repo, tid, pid)
	dup, err := repo.Duplicate(ctx, tid, src.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.Number != "EST-0002" || dup.Status != "draft" || len(dup.LineItems) != 1 {
		t.Fatalf("Duplicate = %+v", dup)
	}
}

func TestEstimateListAndDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewEstimates(conn)
	ctx := context.Background()

	est := mkEstimate(t, repo, tid, pid)
	if list, err := repo.List(ctx, tid); err != nil || len(list) != 1 {
		t.Fatalf("List len=%d err=%v", len(list), err)
	}
	if err := repo.Delete(ctx, tid, est.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, est.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestEstimateTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	pidA := seedParticipant(t, conn, a, "A Jane")
	repo := NewEstimates(conn)
	ctx := context.Background()

	est := mkEstimate(t, repo, a, pidA)
	if got, _ := repo.Get(ctx, b, est.ID); got != nil {
		t.Fatalf("tenant B read tenant A's estimate: %+v", got)
	}
	if list, _ := repo.List(ctx, b); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
}
