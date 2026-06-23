package pricelist

import (
	"context"
	"testing"
)

func capPtr(f float64) *float64 { return &f }

// TestIngestClosesPriorVersionWindow verifies that ingesting a new price-list
// version closes the prior open version the day before the new effective_from, so
// date-windows never overlap and a historical service date resolves to the
// version that was effective then (not the newest).
func TestIngestClosesPriorVersionWindow(t *testing.T) {
	conn := newTestDB(t) // per-tenant price list, empty by default
	repo := NewItems(conn)
	ctx := context.Background()
	item := []ImportItem{{Code: "X", Name: "X", Taxable: false, Prices: map[string]*float64{"national": capPtr(10)}}}

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
