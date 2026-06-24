package pricelist

import (
	"context"
	"testing"
)

func TestCatalogGetVersion(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)
	cap := 10.0
	vID, _ := seedCatalog(t, conn, tid, "v1", "2025-07-01", "", "X", &cap)

	got, err := repo.GetVersion(ctx, tid, vID)
	if err != nil || got == nil || got.ID != vID || got.Label != "v1" {
		t.Fatalf("GetVersion = %+v err=%v", got, err)
	}
	// Absent id returns (nil, nil).
	missing, err := repo.GetVersion(ctx, tid, "no-such-version")
	if err != nil || missing != nil {
		t.Fatalf("GetVersion missing = %+v err=%v, want nil/nil", missing, err)
	}
}

func TestCatalogSearchItems(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)
	cap := 50.0
	vID, _ := seedCatalog(t, conn, tid, "v", "2025-07-01", "", "01_011_0107_1_1", &cap)

	// Match by code substring.
	byCode, err := repo.SearchItems(ctx, tid, vID, "0107")
	if err != nil {
		t.Fatalf("SearchItems code: %v", err)
	}
	if len(byCode) != 1 {
		t.Fatalf("search by code = %d, want 1", len(byCode))
	}
	// Empty query matches everything in the version.
	all, err := repo.SearchItems(ctx, tid, vID, "")
	if err != nil {
		t.Fatalf("SearchItems empty: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("search empty = %d, want 1", len(all))
	}
	// No match.
	none, err := repo.SearchItems(ctx, tid, vID, "zzz")
	if err != nil {
		t.Fatalf("SearchItems none: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("search zzz = %d, want 0", len(none))
	}
}

func TestCatalogIngest(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)

	// Validation failures.
	if _, err := repo.Ingest(ctx, tid, "", "2025-07-01", "f.csv", []ImportItem{{Code: "X"}}); err == nil {
		t.Fatal("Ingest empty label: want error")
	}
	if _, err := repo.Ingest(ctx, tid, "v", "", "f.csv", []ImportItem{{Code: "X"}}); err == nil {
		t.Fatal("Ingest empty effectiveFrom: want error")
	}
	if _, err := repo.Ingest(ctx, tid, "v", "2025-07-01", "f.csv", nil); err == nil {
		t.Fatal("Ingest no rows: want error")
	}

	price := 100.0
	res, err := repo.Ingest(ctx, tid, "2025-26", "2025-07-01", "prices.csv", []ImportItem{
		{Code: "01_011_0107_1_1", Name: "Support A", Unit: "H", Taxable: false, UnitPrice: &price},
		{Code: "01_011_0107_8_1", Name: "Support B (free-form)"},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if res.Version == nil || res.ItemCount != 2 {
		t.Fatalf("Ingest result = %+v, want 2 items", res)
	}

	// The created version is resolvable and carries the ingested items.
	items, err := repo.ListItems(ctx, tid, res.Version.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("ListItems after ingest = %d err=%v, want 2", len(items), err)
	}
	// The priced item carries its unit_price; the free-form item has none.
	priced, err := repo.GetItemByCode(ctx, tid, res.Version.ID, "01_011_0107_1_1")
	if err != nil || priced == nil || priced.UnitPrice == nil || *priced.UnitPrice != 100.0 {
		t.Fatalf("priced item = %+v err=%v, want unit_price 100", priced, err)
	}
	freeform, err := repo.GetItemByCode(ctx, tid, res.Version.ID, "01_011_0107_8_1")
	if err != nil || freeform == nil || freeform.UnitPrice != nil {
		t.Fatalf("free-form item = %+v err=%v, want nil unit_price", freeform, err)
	}
}
