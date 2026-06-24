package pricelist

import (
	"testing"
)

// findMatch returns the Match with the given code, or nil.
func findMatch(ms []*Match, code string) *Match {
	for i := range ms { // bounded by len(ms)
		if ms[i].Code == code {
			return ms[i]
		}
	}
	return nil
}

func TestSearchForDateByKeywordAttachesUnitPrice(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	svc := NewService(conn)

	// Item names are "Item <code>", so the keyword matches by name.
	ms, err := svc.SearchForDate(tctx(tid), "01_011", "2026-01-15", 0)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	m := findMatch(ms, "01_011")
	if m == nil {
		t.Fatalf("expected a match for 01_011, got %+v", ms)
	}
	if m.UnitPrice == nil || *m.UnitPrice != 100 {
		t.Fatalf("unit price not attached: %+v", m.UnitPrice)
	}
	if m.VersionID == "" {
		t.Fatalf("match meta = %+v", m)
	}
}

func TestSearchForDatePartialCode(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_011_0107_1_1", true, 60)
	svc := NewService(conn)

	ms, err := svc.SearchForDate(tctx(tid), "01_011", "2026-01-15", 0)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	if findMatch(ms, "01_011_0107_1_1") == nil {
		t.Fatalf("partial code search should match, got %+v", ms)
	}
}

func TestSearchForDateOutsideWindowEmpty(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_011", true, 100)
	svc := NewService(conn)

	// Service date before any catalogue window.
	ms, err := svc.SearchForDate(tctx(tid), "01_011", "2020-01-01", 0)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	if ms == nil || len(ms) != 0 {
		t.Fatalf("out-of-window must return non-nil empty slice, got %+v", ms)
	}
}

func TestSearchForDateEmptyDateErrors(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	svc := NewService(conn)

	if _, err := svc.SearchForDate(tctx(tid), "01_011", "", 0); err == nil {
		t.Fatal("empty service date must error")
	}
}

func TestSearchForDateLimitCaps(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	verID := seedUnitPricedItem(t, conn, tid, "v1", "2025-07-01", "2026-06-30", "01_001", true, 10)
	addUnitPricedItemToVersion(t, conn, tid, verID, "01_002", true, 10)
	addUnitPricedItemToVersion(t, conn, tid, verID, "01_003", true, 10)
	svc := NewService(conn)

	// Broad keyword "01_" matches all three; limit caps to 2.
	ms, err := svc.SearchForDate(tctx(tid), "01_", "2026-01-15", 2)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("limit=2 should cap results, got %d: %+v", len(ms), ms)
	}
}
