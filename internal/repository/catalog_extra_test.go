package repository

import (
	"context"
	"testing"
)

func TestCatalogGetVersion(t *testing.T) {
	conn := newTestDB(t)
	repo := NewCatalog(conn)
	ctx := context.Background()
	cap := 10.0
	vID, _ := seedCatalog(t, conn, "v1", "2025-07-01", "", "X", &cap)

	got, err := repo.GetVersion(ctx, vID)
	if err != nil || got == nil || got.ID != vID || got.Label != "v1" {
		t.Fatalf("GetVersion = %+v err=%v", got, err)
	}
	// Absent id returns (nil, nil).
	missing, err := repo.GetVersion(ctx, 999999)
	if err != nil || missing != nil {
		t.Fatalf("GetVersion missing = %+v err=%v, want nil/nil", missing, err)
	}
}

func TestCatalogSearchSupportItems(t *testing.T) {
	conn := newTestDB(t)
	repo := NewCatalog(conn)
	ctx := context.Background()
	cap := 50.0
	vID, _ := seedCatalog(t, conn, "v", "2025-07-01", "", "01_011_0107_1_1", &cap)

	// Match by code substring.
	byCode, err := repo.SearchSupportItems(ctx, vID, "0107")
	if err != nil {
		t.Fatalf("SearchSupportItems code: %v", err)
	}
	if len(byCode) != 1 {
		t.Fatalf("search by code = %d, want 1", len(byCode))
	}
	// Empty query matches everything in the version.
	all, err := repo.SearchSupportItems(ctx, vID, "")
	if err != nil {
		t.Fatalf("SearchSupportItems empty: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("search empty = %d, want 1", len(all))
	}
	// No match.
	none, err := repo.SearchSupportItems(ctx, vID, "zzz")
	if err != nil {
		t.Fatalf("SearchSupportItems none: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("search zzz = %d, want 0", len(none))
	}
}

func TestCatalogListPrices(t *testing.T) {
	conn := newTestDB(t)
	repo := NewCatalog(conn)
	ctx := context.Background()
	cap := 75.0
	_, itemID := seedCatalog(t, conn, "v", "2025-07-01", "", "X", &cap)

	prices, err := repo.ListPrices(ctx, itemID)
	if err != nil {
		t.Fatalf("ListPrices: %v", err)
	}
	if len(prices) != 1 || prices[0].Zone != "national" || prices[0].PriceCap == nil || *prices[0].PriceCap != 75.0 {
		t.Fatalf("ListPrices = %+v, want one national cap 75", prices)
	}
}

func TestCatalogIngest(t *testing.T) {
	conn := newTestDB(t)
	repo := NewCatalog(conn)
	ctx := context.Background()

	// Validation failures.
	if _, err := repo.Ingest(ctx, "", "2025-07-01", "f.csv", []IngestItem{{Code: "X"}}); err == nil {
		t.Fatal("Ingest empty label: want error")
	}
	if _, err := repo.Ingest(ctx, "v", "", "f.csv", []IngestItem{{Code: "X"}}); err == nil {
		t.Fatal("Ingest empty effectiveFrom: want error")
	}
	if _, err := repo.Ingest(ctx, "v", "2025-07-01", "f.csv", nil); err == nil {
		t.Fatal("Ingest no rows: want error")
	}

	cap := 100.0
	res, err := repo.Ingest(ctx, "2025-26", "2025-07-01", "prices.csv", []IngestItem{
		{Code: "01_011_0107_1_1", Name: "Support A", Unit: "H", GstFree: true,
			Prices: map[string]*float64{"national": &cap}},
		{Code: "01_011_0107_8_1", Name: "Support B (quotable)",
			Prices: map[string]*float64{"national": nil}},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if res.Version == nil || res.ItemCount != 2 || res.PriceCount != 2 {
		t.Fatalf("Ingest result = %+v, want 2 items / 2 prices", res)
	}

	// The created version is resolvable and carries the ingested items.
	items, err := repo.ListSupportItems(ctx, res.Version.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("ListSupportItems after ingest = %d err=%v, want 2", len(items), err)
	}
	// Quotable item resolves with a nil price cap.
	price, err := repo.ResolveZonePrice(ctx, res.Version.ID, "01_011_0107_8_1", "national")
	if err != nil || price == nil {
		t.Fatalf("ResolveZonePrice quotable = %+v err=%v", price, err)
	}
	if price.PriceCap != nil {
		t.Fatalf("quotable PriceCap = %v, want nil", *price.PriceCap)
	}
}
