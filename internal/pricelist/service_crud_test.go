package pricelist

import (
	"context"
	"testing"
)

// TestSupportCatalogGetVersionAndListItems seeds a small priced version via the
// repo then exercises the read-only GetVersion + ListItems methods.
func TestSupportCatalogGetVersionAndListItems(t *testing.T) {
	conn := newTestDB(t)
	read := NewService(conn)
	ctx := context.Background()

	versionID := seedUnitPricedItem(t, conn, "v1", "2025-07-01", "", "01_011_0107_1_1", true, 67.56)

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
	if items[0].UnitPrice == nil || *items[0].UnitPrice != 67.56 {
		t.Fatalf("item UnitPrice = %v, want 67.56", items[0].UnitPrice)
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
