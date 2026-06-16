package repository

import (
	"context"
	"testing"
	"time"
)

func mkTemplate(t *testing.T, repo *RecurringRepo, tid, pid int64, nextDue string) *RecurringTemplate {
	t.Helper()
	tpl, err := repo.Create(context.Background(), tid, RecurringInput{
		ParticipantID: &pid,
		Name:          "Weekly support",
		Frequency:     "weekly",
		NextDue:       nextDue,
		TaxRate:       10,
		LineItems:     []RecurringLine{{Description: "Support", Quantity: 1, UnitPrice: 100}},
		IsActive:      true,
	})
	if err != nil {
		t.Fatalf("Create template: %v", err)
	}
	return tpl
}

func TestRecurringCRUD(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewRecurring(conn)
	ctx := context.Background()

	tpl := mkTemplate(t, repo, tid, pid, "2026-01-01")
	if tpl.ID == 0 || len(tpl.LineItems) != 1 || tpl.ParticipantName != "Jane" {
		t.Fatalf("Create = %+v", tpl)
	}
	up, err := repo.Update(ctx, tid, tpl.ID, RecurringInput{
		ParticipantID: &pid, Name: "Monthly", Frequency: "monthly", NextDue: "2026-02-01",
		TaxRate: 0, LineItems: []RecurringLine{{Description: "X", Quantity: 1, UnitPrice: 10}}, IsActive: true,
	})
	if err != nil || up.Frequency != "monthly" || up.Name != "Monthly" {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if list, _ := repo.List(ctx, tid, false); len(list) != 1 {
		t.Fatalf("List len=%d, want 1", len(list))
	}
	if err := repo.Delete(ctx, tid, tpl.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, tpl.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestRecurringValidation(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewRecurring(conn)
	ctx := context.Background()
	pid := int64(1)
	if _, err := repo.Create(ctx, tid, RecurringInput{Name: "", ParticipantID: &pid, Frequency: "weekly", NextDue: "2026-01-01"}); err == nil {
		t.Fatal("empty name: want error")
	}
	if _, err := repo.Create(ctx, tid, RecurringInput{Name: "X", ParticipantID: &pid, Frequency: "daily", NextDue: "2026-01-01"}); err == nil {
		t.Fatal("bad frequency: want error")
	}
}

func TestRecurringGenerateOneAdvancesNextDue(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewRecurring(conn)
	ctx := context.Background()

	tpl := mkTemplate(t, repo, tid, pid, "2026-01-01")
	inv, err := repo.GenerateOne(ctx, tid, tpl.ID)
	if err != nil {
		t.Fatalf("GenerateOne: %v", err)
	}
	if inv == nil || inv.Number != "INV-0001" {
		t.Fatalf("generated invoice = %+v", inv)
	}
	// tax_rate 10% on subtotal 100 → tax 10, total 110.
	if inv.Subtotal != 100 || inv.Tax != 10 || inv.Total != 110 {
		t.Fatalf("totals = %.2f/%.2f/%.2f, want 100/10/110", inv.Subtotal, inv.Tax, inv.Total)
	}
	got, _ := repo.Get(ctx, tid, tpl.ID)
	if got.NextDue != "2026-01-08" {
		t.Fatalf("NextDue = %q, want 2026-01-08 (advanced one week)", got.NextDue)
	}
}

func TestRecurringGenerateDue(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := NewRecurring(conn)
	ctx := context.Background()

	// Past due date so it is selected.
	past := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	mkTemplate(t, repo, tid, pid, past)

	gen1, err := repo.GenerateDue(ctx)
	if err != nil {
		t.Fatalf("GenerateDue: %v", err)
	}
	if len(gen1) != 1 {
		t.Fatalf("GenerateDue produced %d, want 1", len(gen1))
	}
	// Idempotent: re-running finds nothing due (next_due advanced into future).
	gen2, err := repo.GenerateDue(ctx)
	if err != nil {
		t.Fatalf("GenerateDue 2: %v", err)
	}
	if len(gen2) != 0 {
		t.Fatalf("re-run GenerateDue produced %d, want 0 (idempotent)", len(gen2))
	}
}

func TestRecurringTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	pidA := seedParticipant(t, conn, a, "A Jane")
	repo := NewRecurring(conn)
	ctx := context.Background()

	tpl := mkTemplate(t, repo, a, pidA, "2026-01-01")
	if got, _ := repo.Get(ctx, b, tpl.ID); got != nil {
		t.Fatalf("tenant B read tenant A's template: %+v", got)
	}
	if list, _ := repo.List(ctx, b, false); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
}
