package repository

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func newTaxRatesRepo(t *testing.T) *TaxRatesRepo {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "tr.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewTaxRates(conn)
}

func TestTaxRateCreate(t *testing.T) {
	repo := newTaxRatesRepo(t)
	ctx := context.Background()

	tr, err := repo.Create(ctx, TaxRateInput{Name: "GST", Rate: 0.1, IsDefault: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tr == nil {
		t.Fatal("Create returned nil")
	}
	if tr.ID <= 0 {
		t.Fatalf("ID = %d, want > 0", tr.ID)
	}
	if tr.Name != "GST" || tr.Rate != 0.1 {
		t.Fatalf("Create = %+v, want GST/0.1", tr)
	}
	if !tr.IsDefault {
		t.Fatalf("IsDefault = false, want true")
	}
}

func TestTaxRateCreateRejectsEmptyName(t *testing.T) {
	repo := newTaxRatesRepo(t)
	if _, err := repo.Create(context.Background(), TaxRateInput{Name: "", Rate: 0.1}); err == nil {
		t.Fatal("Create with empty name: want error, got nil")
	}
}

func TestTaxRateExclusiveDefaultOnCreate(t *testing.T) {
	repo := newTaxRatesRepo(t)
	ctx := context.Background()

	first, err := repo.Create(ctx, TaxRateInput{Name: "GST", Rate: 0.1, IsDefault: true})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := repo.Create(ctx, TaxRateInput{Name: "VAT", Rate: 0.2, IsDefault: true})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	// The second is now the sole default.
	def, err := repo.GetDefault(ctx)
	if err != nil {
		t.Fatalf("GetDefault: %v", err)
	}
	if def == nil || def.ID != second.ID {
		t.Fatalf("GetDefault = %+v, want id=%d (second)", def, second.ID)
	}

	// The first is no longer default.
	got, err := repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("Get first: %v", err)
	}
	if got == nil || got.IsDefault {
		t.Fatalf("first IsDefault = %+v, want false", got)
	}

	// Exactly one row has IsDefault true.
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	count := 0
	for _, r := range list {
		if r.IsDefault {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("default count = %d, want 1", count)
	}
}

func TestTaxRateCreateNonDefaultKeepsExisting(t *testing.T) {
	repo := newTaxRatesRepo(t)
	ctx := context.Background()

	first, err := repo.Create(ctx, TaxRateInput{Name: "GST", Rate: 0.1, IsDefault: true})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if _, err := repo.Create(ctx, TaxRateInput{Name: "VAT", Rate: 0.2, IsDefault: false}); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	def, err := repo.GetDefault(ctx)
	if err != nil {
		t.Fatalf("GetDefault: %v", err)
	}
	if def == nil || def.ID != first.ID {
		t.Fatalf("GetDefault = %+v, want id=%d (first unchanged)", def, first.ID)
	}
}

func TestTaxRateListOrdered(t *testing.T) {
	repo := newTaxRatesRepo(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, TaxRateInput{Name: "Beta", Rate: 0.05}); err != nil {
		t.Fatalf("Create Beta: %v", err)
	}
	if _, err := repo.Create(ctx, TaxRateInput{Name: "Alpha", Rate: 0.05}); err != nil {
		t.Fatalf("Create Alpha: %v", err)
	}
	if _, err := repo.Create(ctx, TaxRateInput{Name: "Zed", Rate: 0.2, IsDefault: true}); err != nil {
		t.Fatalf("Create Zed: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// is_default DESC first (Zed), then name ascending.
	want := []string{"Zed", "Alpha", "Beta"}
	if len(list) != len(want) {
		t.Fatalf("List len = %d, want %d", len(list), len(want))
	}
	for i := range want {
		if list[i].Name != want[i] {
			t.Fatalf("list[%d].Name = %q, want %q", i, list[i].Name, want[i])
		}
	}
}

func TestTaxRateGetDefaultEmpty(t *testing.T) {
	repo := newTaxRatesRepo(t)
	def, err := repo.GetDefault(context.Background())
	if err != nil {
		t.Fatalf("GetDefault empty: %v", err)
	}
	if def != nil {
		t.Fatalf("GetDefault empty = %+v, want nil", def)
	}
}

func TestTaxRateGet(t *testing.T) {
	repo := newTaxRatesRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, TaxRateInput{Name: "GST", Rate: 0.1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "GST" {
		t.Fatalf("Get = %+v, want Name=GST", got)
	}

	missing, err := repo.Get(ctx, 99999)
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Get missing = %+v, want nil", missing)
	}
}

func TestTaxRateUpdateBecomesSoleDefault(t *testing.T) {
	repo := newTaxRatesRepo(t)
	ctx := context.Background()

	first, err := repo.Create(ctx, TaxRateInput{Name: "GST", Rate: 0.1, IsDefault: true})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := repo.Create(ctx, TaxRateInput{Name: "VAT", Rate: 0.2, IsDefault: false})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	updated, err := repo.Update(ctx, second.ID, TaxRateInput{Name: "VAT", Rate: 0.25, IsDefault: true})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil || !updated.IsDefault || updated.Rate != 0.25 {
		t.Fatalf("Update = %+v, want IsDefault=true, Rate=0.25", updated)
	}

	def, err := repo.GetDefault(ctx)
	if err != nil {
		t.Fatalf("GetDefault: %v", err)
	}
	if def == nil || def.ID != second.ID {
		t.Fatalf("GetDefault = %+v, want id=%d", def, second.ID)
	}

	got, err := repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("Get first: %v", err)
	}
	if got == nil || got.IsDefault {
		t.Fatalf("first IsDefault = %+v, want cleared", got)
	}
}

func TestTaxRateUpdateMissing(t *testing.T) {
	repo := newTaxRatesRepo(t)
	got, err := repo.Update(context.Background(), 99999, TaxRateInput{Name: "X", Rate: 0.1})
	if err != nil {
		t.Fatalf("Update missing: %v", err)
	}
	if got != nil {
		t.Fatalf("Update missing = %+v, want nil", got)
	}
}

func TestTaxRateUpdateRejectsEmptyName(t *testing.T) {
	repo := newTaxRatesRepo(t)
	if _, err := repo.Update(context.Background(), 1, TaxRateInput{Name: ""}); err == nil {
		t.Fatal("Update with empty name: want error, got nil")
	}
}

func TestTaxRateDelete(t *testing.T) {
	repo := newTaxRatesRepo(t)
	ctx := context.Background()

	tr, err := repo.Create(ctx, TaxRateInput{Name: "GST", Rate: 0.1})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, tr.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := repo.Get(ctx, tr.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("row still present after delete: %+v", got)
	}
}
