package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// --- CatalogRepo (global NDIS catalogue) ---

// seedCatalog inserts a version with one priced support item and returns ids.
func seedCatalog(t *testing.T, conn *sql.DB, label, from, to, code string, cap *float64) (versionID, itemID int64) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	now := time.Now().UTC().Format(time.RFC3339)
	var et sql.NullString
	if to != "" {
		et = sql.NullString{String: to, Valid: true}
	}
	v, err := q.CreateCatalogVersion(ctx, gen.CreateCatalogVersionParams{
		Uuid: uuid.NewString(), Label: label, EffectiveFrom: from, EffectiveTo: et, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateCatalogVersion: %v", err)
	}
	si, err := q.CreateSupportItem(ctx, gen.CreateSupportItemParams{
		Uuid: uuid.NewString(), CatalogVersionID: v.ID, Code: code, Name: "Item " + code, GstFree: 1,
	})
	if err != nil {
		t.Fatalf("CreateSupportItem: %v", err)
	}
	var pc sql.NullFloat64
	if cap != nil {
		pc = sql.NullFloat64{Float64: *cap, Valid: true}
	}
	if _, err := q.CreateSupportItemPrice(ctx, gen.CreateSupportItemPriceParams{
		SupportItemID: si.ID, Zone: "national", PriceCap: pc,
	}); err != nil {
		t.Fatalf("CreateSupportItemPrice: %v", err)
	}
	return v.ID, si.ID
}

func TestCatalogResolveVersionForDate(t *testing.T) {
	conn := newTestDB(t)
	repo := NewCatalog(conn)
	ctx := context.Background()
	cap := 100.0
	vID, _ := seedCatalog(t, conn, "2025-26", "2025-07-01", "2026-06-30", "01_011_0107_1_1", &cap)

	// Date inside the window resolves.
	v, err := repo.ResolveVersionForDate(ctx, "2026-01-15")
	if err != nil || v == nil || v.ID != vID {
		t.Fatalf("ResolveVersionForDate inside = %+v err=%v", v, err)
	}
	// Date before the window resolves to nil.
	v, err = repo.ResolveVersionForDate(ctx, "2025-01-01")
	if err != nil {
		t.Fatalf("ResolveVersionForDate before: %v", err)
	}
	if v != nil {
		t.Fatalf("ResolveVersionForDate before window = %+v, want nil", v)
	}
}

func TestCatalogGetByCodeAndZonePrice(t *testing.T) {
	conn := newTestDB(t)
	repo := NewCatalog(conn)
	ctx := context.Background()
	cap := 193.99
	vID, itemID := seedCatalog(t, conn, "v", "2025-07-01", "", "01_011_0107_1_1", &cap)

	si, err := repo.GetSupportItemByCode(ctx, vID, "01_011_0107_1_1")
	if err != nil || si == nil || si.ID != itemID {
		t.Fatalf("GetSupportItemByCode = %+v err=%v", si, err)
	}
	if !si.GstFree {
		t.Fatalf("expected GstFree true")
	}

	price, err := repo.ResolveZonePrice(ctx, vID, "01_011_0107_1_1", "national")
	if err != nil || price == nil || price.PriceCap == nil || *price.PriceCap != 193.99 {
		t.Fatalf("ResolveZonePrice = %+v err=%v", price, err)
	}
}

func TestCatalogQuotablePriceCapNil(t *testing.T) {
	conn := newTestDB(t)
	repo := NewCatalog(conn)
	ctx := context.Background()
	vID, _ := seedCatalog(t, conn, "v", "2025-07-01", "", "01_011_0107_8_1", nil)

	price, err := repo.ResolveZonePrice(ctx, vID, "01_011_0107_8_1", "national")
	if err != nil || price == nil {
		t.Fatalf("ResolveZonePrice = %+v err=%v", price, err)
	}
	if price.PriceCap != nil {
		t.Fatalf("quotable item PriceCap = %v, want nil", *price.PriceCap)
	}
}

func TestCatalogListVersionsAndItems(t *testing.T) {
	conn := newTestDB(t)
	repo := NewCatalog(conn)
	ctx := context.Background()
	cap := 50.0
	vID, _ := seedCatalog(t, conn, "v", "2025-07-01", "", "X", &cap)

	if vs, err := repo.ListVersions(ctx); err != nil || len(vs) != 1 {
		t.Fatalf("ListVersions len=%d err=%v", len(vs), err)
	}
	if items, err := repo.ListSupportItems(ctx, vID); err != nil || len(items) != 1 {
		t.Fatalf("ListSupportItems len=%d err=%v", len(items), err)
	}
}
