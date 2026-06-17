package repository

import (
	"context"
	"testing"
)

func TestParticipantBulkDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	a, _ := repo.Create(ctx, tid, ParticipantInput{Name: "Alice"})
	b, _ := repo.Create(ctx, tid, ParticipantInput{Name: "Bob"})
	c, _ := repo.Create(ctx, tid, ParticipantInput{Name: "Carol"})

	// Empty slice is a no-op.
	if err := repo.BulkDelete(ctx, tid, nil); err != nil {
		t.Fatalf("BulkDelete empty: %v", err)
	}
	if err := repo.BulkDelete(ctx, tid, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, _ := repo.List(ctx, tid, "")
	if len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("after bulk delete = %+v, want only Carol (id=%d)", list, c.ID)
	}
}

// TestParticipantListPlain exercises the no-search List path (toParticipantList),
// asserting ordering by name and that fields round-trip.
func TestParticipantListPlain(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	if _, err := repo.Create(ctx, tid, ParticipantInput{Name: "Zoe", NDISNumber: "999"}); err != nil {
		t.Fatalf("Create Zoe: %v", err)
	}
	if _, err := repo.Create(ctx, tid, ParticipantInput{Name: "Amy", Email: "amy@x.com"}); err != nil {
		t.Fatalf("Create Amy: %v", err)
	}

	list, err := repo.List(ctx, tid, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	// Ordered by name: Amy before Zoe.
	if list[0].Name != "Amy" || list[1].Name != "Zoe" {
		t.Fatalf("order = [%q, %q], want [Amy, Zoe]", list[0].Name, list[1].Name)
	}
	if list[0].Email != "amy@x.com" {
		t.Fatalf("Amy email = %q, want amy@x.com", list[0].Email)
	}
}
