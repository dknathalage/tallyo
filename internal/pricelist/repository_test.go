package pricelist

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// --- ItemsRepo (tenant-owned price list) ---

// seedCatalog inserts a version with one item (optionally priced) and returns ids.
func seedCatalog(t *testing.T, conn *sql.DB, tenantID int64, label, from, to, code string, unitPrice *float64) (versionID, itemID int64) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	now := time.Now().UTC().Format(time.RFC3339)
	var et sql.NullString
	if to != "" {
		et = sql.NullString{String: to, Valid: true}
	}
	v, err := q.CreatePriceListVersion(ctx, gen.CreatePriceListVersionParams{
		TenantID: tenantID, Uuid: uuid.NewString(), Label: label, EffectiveFrom: from, EffectiveTo: et, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreatePriceListVersion: %v", err)
	}
	var up sql.NullFloat64
	if unitPrice != nil {
		up = sql.NullFloat64{Float64: *unitPrice, Valid: true}
	}
	si, err := q.CreateItem(ctx, gen.CreateItemParams{
		TenantID: tenantID, Uuid: uuid.NewString(), PriceListVersionID: v.ID, Code: code, Name: "Item " + code, Taxable: 0,
		UnitPrice: up,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	return v.ID, si.ID
}

func TestCatalogResolveVersionForDate(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)
	cap := 100.0
	vID, _ := seedCatalog(t, conn, tid, "2025-26", "2025-07-01", "2026-06-30", "01_011_0107_1_1", &cap)

	// Date inside the window resolves.
	v, err := repo.ResolveVersionForDate(ctx, tid, "2026-01-15")
	if err != nil || v == nil || v.ID != vID {
		t.Fatalf("ResolveVersionForDate inside = %+v err=%v", v, err)
	}
	// Date before the window resolves to nil.
	v, err = repo.ResolveVersionForDate(ctx, tid, "2025-01-01")
	if err != nil {
		t.Fatalf("ResolveVersionForDate before: %v", err)
	}
	if v != nil {
		t.Fatalf("ResolveVersionForDate before window = %+v, want nil", v)
	}
}

func TestCatalogGetByCode(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)
	price := 193.99
	vID, itemID := seedCatalog(t, conn, tid, "v", "2025-07-01", "", "01_011_0107_1_1", &price)

	si, err := repo.GetItemByCode(ctx, tid, vID, "01_011_0107_1_1")
	if err != nil || si == nil || si.ID != itemID {
		t.Fatalf("GetItemByCode = %+v err=%v", si, err)
	}
	if si.Taxable {
		t.Fatalf("expected Taxable false (GST-free item)")
	}
	if si.UnitPrice == nil || *si.UnitPrice != 193.99 {
		t.Fatalf("GetItemByCode UnitPrice = %v, want 193.99", si.UnitPrice)
	}
}

func TestCatalogListVersionsAndItems(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)
	cap := 50.0
	vID, _ := seedCatalog(t, conn, tid, "v", "2025-07-01", "", "X", &cap)

	if vs, err := repo.ListVersions(ctx, tid); err != nil || len(vs) != 1 {
		t.Fatalf("ListVersions len=%d err=%v", len(vs), err)
	}
	if items, err := repo.ListItems(ctx, tid, vID); err != nil || len(items) != 1 {
		t.Fatalf("ListItems len=%d err=%v", len(items), err)
	}
}
