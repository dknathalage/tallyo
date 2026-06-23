package billing

import (
	"context"
	"testing"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// TestValidatePinnedVersionNotRepriced is the core guard for the requirement that
// a NEW catalogue version must never re-price an EXISTING invoice. A line that
// already carries a pinned PriceListVersionID (an edited/existing line) validates
// against THAT version (and is filled from that version's unit_price), not
// whatever version is current for the service date. A fresh (unpinned) line
// resolves to the current version.
func TestValidatePinnedVersionNotRepriced(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedClient(t, conn, tid)

	// v1: unit_price 100 for code 99_test. v2 (later effective, overlapping): 50.
	v1 := seedUnitPricedItem(t, conn, "v1", "2025-01-01", "2025-12-31", "99_test", true, 100)
	seedUnitPricedItem(t, conn, "v2", "2025-06-01", "2026-12-31", "99_test", true, 50)

	val := NewLineValidator(conn)
	ctx := context.Background()

	// Lines pin the price-list version by its UUID (the tenant reference), not
	// its integer id.
	v1ver, err := gen.New(conn).GetPriceListVersion(ctx, v1)
	if err != nil {
		t.Fatalf("get v1 uuid: %v", err)
	}
	v1uuid := v1ver.Uuid

	// Existing line pinned to v1, caller supplies no price → fills from v1's
	// unit_price (100), staying on v1 even though v2 is current for that date.
	pinned := supportLine("99_test", "2025-07-01", 1, 0)
	pinned.PriceListVersionID = &v1uuid
	res, err := val.Validate(ctx, tid, pid, []LineItemInput{pinned})
	if err != nil {
		t.Fatalf("pinned-to-v1 line should pass: %v", err)
	}
	got := res.Items[0].PriceListVersionID
	if got == nil || *got != v1uuid {
		t.Fatalf("pinned version must be preserved: got %v want %s", got, v1uuid)
	}
	if res.Items[0].UnitPrice != 100 {
		t.Fatalf("pinned line must price from v1 unit_price: got %v want 100", res.Items[0].UnitPrice)
	}

	// Same line UNpinned, no caller price: resolves to the current version v2 and
	// fills from its unit_price (50).
	fresh := supportLine("99_test", "2025-07-01", 1, 0)
	res, err = val.Validate(ctx, tid, pid, []LineItemInput{fresh})
	if err != nil {
		t.Fatalf("unpinned line should resolve v2: %v", err)
	}
	if res.Items[0].UnitPrice != 50 {
		t.Fatalf("unpinned line must price from current v2 unit_price: got %v want 50", res.Items[0].UnitPrice)
	}
}
