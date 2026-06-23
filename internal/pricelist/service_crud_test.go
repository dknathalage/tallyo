package pricelist

import (
	"context"
	"testing"

	"github.com/dknathalage/tallyo/internal/realtime"
)

// TestSupportCatalogGetVersionAndListPrices ingests a small catalogue then
// exercises the read-only GetVersion + ListPrices methods.
func TestSupportCatalogGetVersionAndListPrices(t *testing.T) {
	conn := newTestDB(t)
	hub := realtime.NewHub()
	ingest := NewIngestService(conn, hub)
	read := NewService(conn)
	ctx := context.Background()

	data := catalogXLSX(t, catalogHeaders, [][]string{
		{"01_011_0107_1_1", "Assistance With Self-Care", "Hour", "Core", "Daily Living", "$67.56", "$94.58", "$101.34"},
	})
	summary, err := ingest.IngestXLSX(ctx, data, "v1", "2025-07-01", "c.xlsx")
	if err != nil {
		t.Fatalf("IngestXLSX: %v", err)
	}

	ver, err := read.GetVersion(ctx, summary.VersionID)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if ver == nil || ver.ID != summary.VersionID {
		t.Fatalf("GetVersion = %+v, want id %d", ver, summary.VersionID)
	}

	items, err := read.ListItemsByVersionUUID(ctx, summary.VersionUUID)
	if err != nil {
		t.Fatalf("ListItemsByVersionUUID: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].PriceListVersionUID != summary.VersionUUID {
		t.Fatalf("item PriceListVersionUID = %q, want %q", items[0].PriceListVersionUID, summary.VersionUUID)
	}

	prices, err := read.ListPricesByItemUUID(ctx, items[0].UUID)
	if err != nil {
		t.Fatalf("ListPricesByItemUUID: %v", err)
	}
	if len(prices) != 3 {
		t.Fatalf("ListPrices = %d, want 3 zone rows", len(prices))
	}
}

// TestSupportCatalogGetVersionMissingReturnsNil asserts an absent version id
// yields (nil, nil) rather than an error.
func TestSupportCatalogGetVersionMissingReturnsNil(t *testing.T) {
	conn := newTestDB(t)
	read := NewService(conn)

	ver, err := read.GetVersion(context.Background(), 999999)
	if err != nil {
		t.Fatalf("GetVersion missing: unexpected err %v", err)
	}
	if ver != nil {
		t.Fatalf("GetVersion missing = %+v, want nil", ver)
	}
}
