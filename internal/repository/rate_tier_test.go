package repository

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func newRateTiersRepo(t *testing.T) *RateTiersRepo {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "rt.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewRateTiers(conn)
}

func TestRateTierCreate(t *testing.T) {
	repo := newRateTiersRepo(t)
	ctx := context.Background()

	tier, err := repo.Create(ctx, RateTierInput{Name: "Standard", Description: "d", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tier == nil {
		t.Fatal("Create returned nil tier")
	}
	if tier.ID <= 0 {
		t.Fatalf("ID = %d, want > 0", tier.ID)
	}
	if tier.Name != "Standard" {
		t.Fatalf("Name = %q, want Standard", tier.Name)
	}
	if tier.Description != "d" {
		t.Fatalf("Description = %q, want d", tier.Description)
	}
	if tier.SortOrder != 1 {
		t.Fatalf("SortOrder = %d, want 1", tier.SortOrder)
	}
}

func TestRateTierCreateRejectsEmptyName(t *testing.T) {
	repo := newRateTiersRepo(t)
	if _, err := repo.Create(context.Background(), RateTierInput{Name: ""}); err == nil {
		t.Fatal("Create with empty name: want error, got nil")
	}
}

func TestRateTierListOrdered(t *testing.T) {
	repo := newRateTiersRepo(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, RateTierInput{Name: "B", SortOrder: 2}); err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if _, err := repo.Create(ctx, RateTierInput{Name: "A", SortOrder: 1}); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if _, err := repo.Create(ctx, RateTierInput{Name: "Y", SortOrder: 3}); err != nil {
		t.Fatalf("Create Y: %v", err)
	}
	if _, err := repo.Create(ctx, RateTierInput{Name: "X", SortOrder: 3}); err != nil {
		t.Fatalf("Create X: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"A", "B", "X", "Y"} // sort_order then name
	if len(list) != len(want) {
		t.Fatalf("List len = %d, want %d", len(list), len(want))
	}
	for i := range want {
		if list[i].Name != want[i] {
			t.Fatalf("list[%d].Name = %q, want %q", i, list[i].Name, want[i])
		}
	}
}

func TestRateTierGet(t *testing.T) {
	repo := newRateTiersRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, RateTierInput{Name: "Standard", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "Standard" {
		t.Fatalf("Get = %+v, want Name=Standard", got)
	}

	missing, err := repo.Get(ctx, 99999)
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Get missing = %+v, want nil", missing)
	}
}

func TestRateTierGetDefault(t *testing.T) {
	repo := newRateTiersRepo(t)
	ctx := context.Background()

	empty, err := repo.GetDefault(ctx)
	if err != nil {
		t.Fatalf("GetDefault empty: %v", err)
	}
	if empty != nil {
		t.Fatalf("GetDefault empty = %+v, want nil", empty)
	}

	if _, err := repo.Create(ctx, RateTierInput{Name: "High", SortOrder: 5}); err != nil {
		t.Fatalf("Create High: %v", err)
	}
	if _, err := repo.Create(ctx, RateTierInput{Name: "Low", SortOrder: 1}); err != nil {
		t.Fatalf("Create Low: %v", err)
	}

	def, err := repo.GetDefault(ctx)
	if err != nil {
		t.Fatalf("GetDefault: %v", err)
	}
	if def == nil || def.Name != "Low" {
		t.Fatalf("GetDefault = %+v, want Name=Low (lowest sort_order)", def)
	}
}

func TestRateTierUpdate(t *testing.T) {
	repo := newRateTiersRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, RateTierInput{Name: "Standard", Description: "old", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, RateTierInput{Name: "Premium", Description: "new", SortOrder: 2})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("Update returned nil")
	}
	if updated.Name != "Premium" || updated.Description != "new" || updated.SortOrder != 2 {
		t.Fatalf("Update = %+v, want Premium/new/2", updated)
	}
}

func TestRateTierDeleteWithMultiple(t *testing.T) {
	repo := newRateTiersRepo(t)
	ctx := context.Background()

	a, err := repo.Create(ctx, RateTierInput{Name: "A", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if _, err := repo.Create(ctx, RateTierInput{Name: "B", SortOrder: 2}); err != nil {
		t.Fatalf("Create B: %v", err)
	}

	if err := repo.Delete(ctx, a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := repo.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("row still present after delete: %+v", got)
	}
}

func TestRateTierDeleteLastTierGuard(t *testing.T) {
	repo := newRateTiersRepo(t)
	ctx := context.Background()

	only, err := repo.Create(ctx, RateTierInput{Name: "Only", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = repo.Delete(ctx, only.ID)
	if !errors.Is(err, ErrLastTier) {
		t.Fatalf("Delete last tier err = %v, want ErrLastTier", err)
	}

	got, err := repo.Get(ctx, only.ID)
	if err != nil {
		t.Fatalf("Get after guarded delete: %v", err)
	}
	if got == nil {
		t.Fatal("last tier was removed despite guard")
	}
}

func TestRateTierAuditRows(t *testing.T) {
	repo := newRateTiersRepo(t)
	ctx := context.Background()

	a, err := repo.Create(ctx, RateTierInput{Name: "A", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if _, err := repo.Create(ctx, RateTierInput{Name: "B", SortOrder: 2}); err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if _, err := repo.Update(ctx, a.ID, RateTierInput{Name: "A2", SortOrder: 1}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := repo.Delete(ctx, a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var n int
	if err := repo.db.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='rate_tier'",
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	// create A + create B + update A + delete A = 4; but the test creates two
	// tiers. The spec asks for 3 rate_tier audit rows from create+update+delete
	// on the surviving id. Count the A-related lifecycle plus B's create.
	if n != 4 {
		t.Fatalf("audit rows = %d, want 4 (2 create + update + delete)", n)
	}
}
