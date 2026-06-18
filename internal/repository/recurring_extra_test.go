package repository

import (
	"context"
	"testing"

	"github.com/dknathalage/tallyo/internal/recurring"
)

// TestRecurringListActiveOnly exercises the activeOnly=true path
// (ListActiveRecurringTemplates / activeRowToTemplate), asserting that inactive
// templates are excluded while still appearing in the full list.
func TestRecurringListActiveOnly(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	pid := seedParticipant(t, conn, tid, "Jane")
	repo := recurring.NewRepo(conn)
	ctx := context.Background()

	active := mkTemplate(t, repo, tid, pid, "2026-01-01")

	inactive, err := repo.Create(ctx, tid, recurring.RecurringInput{
		ParticipantID: &pid, Name: "Paused", Frequency: "monthly", NextDue: "2026-02-01",
		TaxRate: 0, LineItems: []recurring.RecurringLine{{Description: "X", Quantity: 1, UnitPrice: 10}}, IsActive: false,
	})
	if err != nil {
		t.Fatalf("Create inactive: %v", err)
	}

	all, err := repo.List(ctx, tid, false)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List all len = %d, want 2", len(all))
	}

	activeList, err := repo.List(ctx, tid, true)
	if err != nil {
		t.Fatalf("List active: %v", err)
	}
	if len(activeList) != 1 || activeList[0].ID != active.ID {
		t.Fatalf("active list = %+v, want only active (id=%d, not %d)", activeList, active.ID, inactive.ID)
	}
	// The active row carries the resolved participant name and parsed line items.
	if activeList[0].ParticipantName != "Jane" || len(activeList[0].LineItems) != 1 {
		t.Fatalf("active row = %+v, want Jane / 1 line item", activeList[0])
	}
}
