package catalog

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func capPtr(f float64) *float64 { return &f }

// TestIngestClosesPriorVersionWindow verifies that ingesting a new catalogue
// version closes the prior open version the day before the new effective_from, so
// date-windows never overlap and a historical service date resolves to the
// version that was effective then (not the newest).
func TestIngestClosesPriorVersionWindow(t *testing.T) {
	conn := newTestDB(t) // clean catalogue (migration seed wiped by helper)
	repo := NewCatalog(conn)
	ctx := context.Background()
	item := []IngestItem{{Code: "X", Name: "X", GstFree: true, Prices: map[string]*float64{"national": capPtr(10)}}}

	if _, err := repo.Ingest(ctx, "v1", "2025-07-01", "f1", item); err != nil {
		t.Fatalf("ingest v1: %v", err)
	}
	if _, err := repo.Ingest(ctx, "v2", "2026-07-01", "f2", item); err != nil {
		t.Fatalf("ingest v2: %v", err)
	}

	// A 2025-26 service date resolves to v1; a 2026-27 date resolves to v2.
	if v, err := repo.ResolveVersionForDate(ctx, "2025-08-01"); err != nil || v == nil || v.Label != "v1" {
		t.Fatalf("2025-08-01 should resolve to v1: got %v err=%v", v, err)
	}
	if v, err := repo.ResolveVersionForDate(ctx, "2026-08-01"); err != nil || v == nil || v.Label != "v2" {
		t.Fatalf("2026-08-01 should resolve to v2: got %v err=%v", v, err)
	}
	// Boundary: the day before v2 still belongs to v1.
	if v, err := repo.ResolveVersionForDate(ctx, "2026-06-30"); err != nil || v == nil || v.Label != "v1" {
		t.Fatalf("2026-06-30 should resolve to v1: got %v err=%v", v, err)
	}
}

// TestMigrationSeedsCatalogue verifies the 00006 migration loads the real NDIS
// 2025-26 catalogue. It opens a migrated DB DIRECTLY (not via newTestDB, which
// wipes the catalogue for the other tests).
func TestMigrationSeedsCatalogue(t *testing.T) {
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	repo := NewCatalog(conn)
	ctx := context.Background()

	ver, err := repo.ResolveVersionForDate(ctx, "2025-08-01")
	if err != nil || ver == nil {
		t.Fatalf("resolve 2025-08-01: got %v err=%v", ver, err)
	}
	if ver.Label != "2025-26" {
		t.Fatalf("seeded version label = %q, want 2025-26", ver.Label)
	}
	items, err := repo.ListSupportItems(ctx, ver.ID)
	if err != nil {
		t.Fatalf("ListSupportItems: %v", err)
	}
	if len(items) < 100 {
		t.Fatalf("seeded catalogue has too few items: %d", len(items))
	}
	// A known real NDIS code is present and resolvable.
	if it, err := repo.GetSupportItemByCode(ctx, ver.ID, "01_011_0107_1_1"); err != nil || it == nil {
		t.Fatalf("expected code 01_011_0107_1_1 in seeded catalogue: got %v err=%v", it, err)
	}
}
