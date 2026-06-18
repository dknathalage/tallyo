package catalog

import (
	"context"
	"testing"
)

// findMatch returns the CatalogMatch with the given code, or nil.
func findMatch(ms []*CatalogMatch, code string) *CatalogMatch {
	for i := range ms { // bounded by len(ms)
		if ms[i].Code == code {
			return ms[i]
		}
	}
	return nil
}

func TestSearchForDateByKeywordAttachesCap(t *testing.T) {
	conn := newTestDB(t)
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true,
		map[string]*float64{"national": fptr(100)})
	svc := NewService(conn)

	// Item names are "Item <code>", so the keyword matches by name.
	ms, err := svc.SearchForDate(context.Background(), "01_011", "2026-01-15", "national", 0)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	m := findMatch(ms, "01_011")
	if m == nil {
		t.Fatalf("expected a match for 01_011, got %+v", ms)
	}
	if m.PriceCap == nil || *m.PriceCap != 100 {
		t.Fatalf("national cap not attached: %+v", m.PriceCap)
	}
	if m.Quotable {
		t.Fatal("capped item must not be Quotable")
	}
	if m.Zone != "national" || m.VersionID == 0 {
		t.Fatalf("match meta = %+v", m)
	}
}

func TestSearchForDatePartialCode(t *testing.T) {
	conn := newTestDB(t)
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011_0107_1_1", true,
		map[string]*float64{"national": fptr(60)})
	svc := NewService(conn)

	ms, err := svc.SearchForDate(context.Background(), "01_011", "2026-01-15", "", 0)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	if findMatch(ms, "01_011_0107_1_1") == nil {
		t.Fatalf("partial code search should match, got %+v", ms)
	}
}

func TestSearchForDateQuotableItem(t *testing.T) {
	conn := newTestDB(t)
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_999", true,
		map[string]*float64{"national": nil})
	svc := NewService(conn)

	ms, err := svc.SearchForDate(context.Background(), "01_999", "2026-01-15", "national", 0)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	m := findMatch(ms, "01_999")
	if m == nil {
		t.Fatalf("expected match for quotable item, got %+v", ms)
	}
	if !m.Quotable || m.PriceCap != nil {
		t.Fatalf("nil-cap item must be Quotable with nil cap: %+v", m)
	}
}

func TestSearchForDateOutsideWindowEmpty(t *testing.T) {
	conn := newTestDB(t)
	seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_011", true,
		map[string]*float64{"national": fptr(100)})
	svc := NewService(conn)

	// Service date before any catalogue window.
	ms, err := svc.SearchForDate(context.Background(), "01_011", "2020-01-01", "national", 0)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	if ms == nil || len(ms) != 0 {
		t.Fatalf("out-of-window must return non-nil empty slice, got %+v", ms)
	}
}

func TestSearchForDateEmptyDateErrors(t *testing.T) {
	conn := newTestDB(t)
	svc := NewService(conn)

	if _, err := svc.SearchForDate(context.Background(), "01_011", "", "national", 0); err == nil {
		t.Fatal("empty service date must error")
	}
}

func TestSearchForDateLimitCaps(t *testing.T) {
	conn := newTestDB(t)
	verID := seedZonedCatalog(t, conn, "v1", "2025-07-01", "2026-06-30", "01_001", true,
		map[string]*float64{"national": fptr(10)})
	addItemToVersion(t, conn, verID, "01_002", true, map[string]*float64{"national": fptr(10)})
	addItemToVersion(t, conn, verID, "01_003", true, map[string]*float64{"national": fptr(10)})
	svc := NewService(conn)

	// Broad keyword "01_" matches all three; limit caps to 2.
	ms, err := svc.SearchForDate(context.Background(), "01_", "2026-01-15", "national", 2)
	if err != nil {
		t.Fatalf("SearchForDate: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("limit=2 should cap results, got %d: %+v", len(ms), ms)
	}
}
