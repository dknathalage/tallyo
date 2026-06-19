package billing

import (
	"context"
	"testing"
)

// TestValidatePinnedVersionNotRepriced is the core guard for the requirement that
// a NEW catalogue version must never re-price an EXISTING invoice. A line that
// already carries a pinned CatalogVersionID (an edited/existing line) validates
// against THAT version's price cap, not whatever version is current for the
// service date. A fresh (unpinned) line resolves to the current version.
func TestValidatePinnedVersionNotRepriced(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	pid := seedParticipantPlan(t, conn, tid, "2025-01-01", "2026-12-31")

	// v1: cap 100 for code 99_test. v2 (later effective, overlapping): cap 50.
	v1 := seedZonedCatalog(t, conn, "v1", "2025-01-01", "2025-12-31", "99_test", true, map[string]*float64{"national": fptr(100)})
	seedZonedCatalog(t, conn, "v2", "2025-06-01", "2026-12-31", "99_test", true, map[string]*float64{"national": fptr(50)})

	val := NewLineValidator(conn)
	ctx := context.Background()

	// Existing line pinned to v1 at $80 (≤ v1 cap 100): must PASS and stay on v1,
	// even though v2 (cap 50) is the current version for that service date.
	pinned := supportLine("99_test", "2025-07-01", 1, 80)
	pinned.CatalogVersionID = &v1
	res, err := val.Validate(ctx, tid, pid, []LineItemInput{pinned})
	if err != nil {
		t.Fatalf("pinned-to-v1 line at 80 (cap 100) should pass: %v", err)
	}
	got := res.Items[0].CatalogVersionID
	if got == nil || *got != v1 {
		t.Fatalf("pinned version must be preserved: got %v want %d", got, v1)
	}

	// Same line UNpinned: resolves to the current version v2 (cap 50) → 80 fails.
	fresh := supportLine("99_test", "2025-07-01", 1, 80)
	if _, err := val.Validate(ctx, tid, pid, []LineItemInput{fresh}); err == nil {
		t.Fatalf("unpinned line at 80 should fail under the current v2 cap of 50")
	}
}
