package customitem

import (
	"context"
	"testing"
)

func TestCustomItemCRUD(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	ci, err := repo.Create(ctx, tid, CustomItemInput{Name: "Travel", Rate: 1.5, Unit: "km", Taxable: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ci.ID == 0 || ci.Rate != 1.5 || !ci.Taxable || ci.Unit != "km" {
		t.Fatalf("Create = %+v", ci)
	}
	got, err := repo.Get(ctx, tid, ci.UUID)
	if err != nil || got == nil || got.Name != "Travel" {
		t.Fatalf("Get = %+v err=%v", got, err)
	}
	up, err := repo.Update(ctx, tid, ci.UUID, CustomItemInput{Name: "Travel2", Rate: 2})
	if err != nil || up == nil || up.Name != "Travel2" || up.Rate != 2 {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if list, _ := repo.List(ctx, tid); len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
	if err := repo.Delete(ctx, tid, ci.UUID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, ci.UUID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestCustomItemTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn)
	b := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	ci, err := repo.Create(ctx, a, CustomItemInput{Name: "A item", Rate: 1})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if got, _ := repo.Get(ctx, b, ci.UUID); got != nil {
		t.Fatalf("tenant B read tenant A's custom item: %+v", got)
	}
	if list, _ := repo.List(ctx, b); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
}

func TestCustomItemSearch(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	if _, err := repo.Create(ctx, tid, CustomItemInput{Name: "Travel km", Rate: 1}); err != nil {
		t.Fatalf("Create Travel: %v", err)
	}
	if _, err := repo.Create(ctx, tid, CustomItemInput{Name: "Cleaning hour", Rate: 2}); err != nil {
		t.Fatalf("Create Cleaning: %v", err)
	}

	hits, err := repo.Search(ctx, tid, "Travel")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Name != "Travel km" {
		t.Fatalf("Search Travel = %+v, want one", hits)
	}
	none, err := repo.Search(ctx, tid, "Nonexistent")
	if err != nil {
		t.Fatalf("Search none: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("Search Nonexistent len = %d, want 0", len(none))
	}
}

func TestCustomItemBulkDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	a, _ := repo.Create(ctx, tid, CustomItemInput{Name: "A", Rate: 1})
	b, _ := repo.Create(ctx, tid, CustomItemInput{Name: "B", Rate: 1})
	c, _ := repo.Create(ctx, tid, CustomItemInput{Name: "C", Rate: 1})

	// Empty is a no-op.
	if err := repo.BulkDelete(ctx, tid, nil); err != nil {
		t.Fatalf("BulkDelete empty: %v", err)
	}
	if err := repo.BulkDelete(ctx, tid, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, _ := repo.List(ctx, tid)
	if len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("after bulk delete = %+v, want only c (id=%d)", list, c.ID)
	}
}
