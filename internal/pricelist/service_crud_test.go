package pricelist

import (
	"context"
	"testing"
)

// TestSupportCatalogGetVersionAndListPrices seeds a small priced version via the
// repo then exercises the read-only GetVersion + ListItems + ListPrices methods.
func TestSupportCatalogGetVersionAndListPrices(t *testing.T) {
	conn := newTestDB(t)
	read := NewService(conn)
	ctx := context.Background()

	versionID := seedZonedCatalog(t, conn, "v1", "2025-07-01", "", "01_011_0107_1_1", true, map[string]*float64{
		"national":    fptr(67.56),
		"remote":      fptr(94.58),
		"very_remote": fptr(101.34),
	})

	ver, err := read.GetVersion(ctx, versionID)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if ver == nil || ver.ID != versionID {
		t.Fatalf("GetVersion = %+v, want id %d", ver, versionID)
	}

	items, err := read.ListItemsByVersionUUID(ctx, ver.UUID)
	if err != nil {
		t.Fatalf("ListItemsByVersionUUID: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].PriceListVersionUID != ver.UUID {
		t.Fatalf("item PriceListVersionUID = %q, want %q", items[0].PriceListVersionUID, ver.UUID)
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
