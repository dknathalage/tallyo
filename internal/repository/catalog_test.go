package repository

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func newCatalogRepo(t *testing.T) *CatalogRepo {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewCatalog(conn)
}

func newCatalogRepoWithTier(t *testing.T) (*CatalogRepo, *RateTiersRepo) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewCatalog(conn), NewRateTiers(conn)
}

func TestCatalogCreate(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	item, err := repo.Create(ctx, CatalogItemInput{Name: "Widget", Rate: 10, Unit: "ea", Category: "Hardware", Sku: "W1", Metadata: ""})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if item == nil {
		t.Fatal("Create returned nil item")
	}
	if item.ID <= 0 {
		t.Fatalf("ID = %d, want > 0", item.ID)
	}
	if item.Name != "Widget" || item.Rate != 10 || item.Unit != "ea" || item.Category != "Hardware" || item.Sku != "W1" {
		t.Fatalf("item = %+v, want Widget/10/ea/Hardware/W1", item)
	}
	if item.Metadata != "{}" {
		t.Fatalf("Metadata = %q, want {}", item.Metadata)
	}
}

func TestCatalogCreateRejectsEmptyName(t *testing.T) {
	repo := newCatalogRepo(t)
	if _, err := repo.Create(context.Background(), CatalogItemInput{Name: "", Rate: 1}); err == nil {
		t.Fatal("Create with empty name: want error, got nil")
	}
}

func TestCatalogListOrdered(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	for _, n := range []string{"Beta", "Alpha", "Gamma"} {
		if _, err := repo.Create(ctx, CatalogItemInput{Name: n, Rate: 1}); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"Alpha", "Beta", "Gamma"}
	if len(list) != len(want) {
		t.Fatalf("List len = %d, want %d", len(list), len(want))
	}
	for i := range want {
		if list[i].Name != want[i] {
			t.Fatalf("list[%d].Name = %q, want %q", i, list[i].Name, want[i])
		}
	}
}

func TestCatalogGet(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, CatalogItemInput{Name: "Widget", Rate: 10})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "Widget" {
		t.Fatalf("Get = %+v, want Name=Widget", got)
	}
	missing, err := repo.Get(ctx, 99999)
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Get missing = %+v, want nil", missing)
	}
}

func TestCatalogUpdate(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, CatalogItemInput{Name: "Widget", Rate: 10, Unit: "ea"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	updated, err := repo.Update(ctx, created.ID, CatalogItemInput{Name: "Gadget", Rate: 20, Unit: "hr", Category: "C", Sku: "G1"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("Update returned nil")
	}
	if updated.Name != "Gadget" || updated.Rate != 20 || updated.Unit != "hr" {
		t.Fatalf("Update = %+v, want Gadget/20/hr", updated)
	}

	missing, err := repo.Update(ctx, 99999, CatalogItemInput{Name: "X", Rate: 1})
	if err != nil {
		t.Fatalf("Update missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Update missing = %+v, want nil", missing)
	}
}

func TestCatalogDelete(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, CatalogItemInput{Name: "Widget", Rate: 10})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("row still present after delete: %+v", got)
	}
}

func TestCatalogBulkDelete(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	a, err := repo.Create(ctx, CatalogItemInput{Name: "A", Rate: 1})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	b, err := repo.Create(ctx, CatalogItemInput{Name: "B", Rate: 1})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if err := repo.BulkDelete(ctx, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List len = %d, want 0", len(list))
	}
}

func TestCatalogSearch(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, CatalogItemInput{Name: "Widget", Rate: 1, Sku: "X1", Category: "Cat"}); err != nil {
		t.Fatalf("Create Widget: %v", err)
	}
	if _, err := repo.Create(ctx, CatalogItemInput{Name: "Thing", Rate: 1, Sku: "wid-sku", Category: "Other"}); err != nil {
		t.Fatalf("Create Thing: %v", err)
	}
	if _, err := repo.Create(ctx, CatalogItemInput{Name: "Unrelated", Rate: 1, Sku: "Z", Category: "WIDcat"}); err != nil {
		t.Fatalf("Create Unrelated: %v", err)
	}

	got, err := repo.Search(ctx, "wid")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// matches by name (Widget), sku (wid-sku), category (WIDcat) -> 3
	if len(got) != 3 {
		t.Fatalf("Search len = %d, want 3 (name/sku/category)", len(got))
	}
}

func TestCatalogCategories(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	for _, c := range []string{"A", "A", "B", ""} {
		if _, err := repo.Create(ctx, CatalogItemInput{Name: "item-" + c, Rate: 1, Category: c}); err != nil {
			t.Fatalf("Create %q: %v", c, err)
		}
	}
	cats, err := repo.Categories(ctx)
	if err != nil {
		t.Fatalf("Categories: %v", err)
	}
	if cats == nil {
		t.Fatal("Categories returned nil slice")
	}
	want := map[string]bool{"A": true, "B": true}
	if len(cats) != len(want) {
		t.Fatalf("Categories = %v, want distinct non-empty [A B]", cats)
	}
	for _, c := range cats {
		if !want[c] {
			t.Fatalf("unexpected category %q in %v", c, cats)
		}
	}
}

func TestCatalogTierRatesUpsert(t *testing.T) {
	repo, tiers := newCatalogRepoWithTier(t)
	ctx := context.Background()

	item, err := repo.Create(ctx, CatalogItemInput{Name: "Widget", Rate: 10})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tier, err := tiers.Create(ctx, RateTierInput{Name: "Standard", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create tier: %v", err)
	}

	if err := repo.SetRate(ctx, item.ID, tier.ID, 7.5); err != nil {
		t.Fatalf("SetRate: %v", err)
	}
	rates, err := repo.GetRates(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetRates: %v", err)
	}
	if len(rates) != 1 || rates[0].RateTierID != tier.ID || rates[0].Rate != 7.5 {
		t.Fatalf("GetRates = %+v, want [{%d 7.5}]", rates, tier.ID)
	}

	// upsert: set again, expect one row with the latest rate.
	if err := repo.SetRate(ctx, item.ID, tier.ID, 9.0); err != nil {
		t.Fatalf("SetRate upsert: %v", err)
	}
	rates, err = repo.GetRates(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetRates after upsert: %v", err)
	}
	if len(rates) != 1 || rates[0].Rate != 9.0 {
		t.Fatalf("GetRates after upsert = %+v, want one row rate 9.0", rates)
	}
}

func TestCatalogEffectiveRate(t *testing.T) {
	repo, tiers := newCatalogRepoWithTier(t)
	ctx := context.Background()

	item, err := repo.Create(ctx, CatalogItemInput{Name: "Widget", Rate: 10})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tier, err := tiers.Create(ctx, RateTierInput{Name: "Standard", SortOrder: 1})
	if err != nil {
		t.Fatalf("Create tier: %v", err)
	}
	other, err := tiers.Create(ctx, RateTierInput{Name: "Premium", SortOrder: 2})
	if err != nil {
		t.Fatalf("Create other tier: %v", err)
	}

	if err := repo.SetRate(ctx, item.ID, tier.ID, 9.0); err != nil {
		t.Fatalf("SetRate: %v", err)
	}

	// tier with a rate -> tier rate
	got, err := repo.EffectiveRate(ctx, item.ID, &tier.ID)
	if err != nil {
		t.Fatalf("EffectiveRate tier: %v", err)
	}
	if got != 9.0 {
		t.Fatalf("EffectiveRate(tier) = %v, want 9.0", got)
	}

	// tier with no rate -> base rate
	got, err = repo.EffectiveRate(ctx, item.ID, &other.ID)
	if err != nil {
		t.Fatalf("EffectiveRate other: %v", err)
	}
	if got != 10 {
		t.Fatalf("EffectiveRate(other) = %v, want 10 (base)", got)
	}

	// nil tier -> base rate
	got, err = repo.EffectiveRate(ctx, item.ID, nil)
	if err != nil {
		t.Fatalf("EffectiveRate nil: %v", err)
	}
	if got != 10 {
		t.Fatalf("EffectiveRate(nil) = %v, want 10 (base)", got)
	}
}
