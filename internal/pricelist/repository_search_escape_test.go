package pricelist

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

// seedNamedItem inserts an item with an explicit name into the
// given version. Used by the LIKE-escaping test where the name (not the code)
// carries the metacharacter under test.
func seedNamedItem(t *testing.T, conn *sql.DB, tenantID, versionID int64, code, name string) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	if _, err := q.CreateItem(ctx, gen.CreateItemParams{
		TenantID:           tenantID,
		Uuid:               uuid.NewString(),
		PriceListVersionID: versionID,
		Code:               code,
		Name:               name,
		Taxable:            0,
	}); err != nil {
		t.Fatalf("CreateItem %q: %v", name, err)
	}
}

func seedBareVersion(t *testing.T, conn *sql.DB, tenantID int64) int64 {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	v, err := gen.New(conn).CreatePriceListVersion(ctx, gen.CreatePriceListVersionParams{
		TenantID: tenantID, Uuid: uuid.NewString(), Label: "v", EffectiveFrom: "2025-07-01", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreatePriceListVersion: %v", err)
	}
	return v.ID
}

// TestCatalogSearchEscapesLikeMetachars asserts that a query containing a LIKE
// metacharacter (here `_`, which would otherwise match any single character) is
// treated literally. With two items "Self_Care" and "SelfXCare", a search for
// "Self_Care" must match ONLY the literal-underscore item.
func TestCatalogSearchEscapesLikeMetachars(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)
	vID := seedBareVersion(t, conn, tid)

	seedNamedItem(t, conn, tid, vID, "CODE_LITERAL", "Self_Care")
	seedNamedItem(t, conn, tid, vID, "CODE_WILDCARD", "SelfXCare")

	got, err := repo.SearchItems(ctx, tid, vID, "Self_Care")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(got) != 1 {
		names := make([]string, len(got))
		for i := range got { // bounded by len(got)
			names[i] = got[i].Name
		}
		t.Fatalf("search %q matched %d items %v, want 1 (only the literal-underscore item)", "Self_Care", len(got), names)
	}
	if got[0].Name != "Self_Care" {
		t.Fatalf("matched %q, want literal %q", got[0].Name, "Self_Care")
	}
}

// TestCatalogSearchEscapesPercent asserts a query containing `%` is literal too.
func TestCatalogSearchEscapesPercent(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)
	vID := seedBareVersion(t, conn, tid)

	seedNamedItem(t, conn, tid, vID, "C1", "100% Cover")
	seedNamedItem(t, conn, tid, vID, "C2", "100 then Cover")

	got, err := repo.SearchItems(ctx, tid, vID, "100% Cover")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(got) != 1 || got[0].Name != "100% Cover" {
		t.Fatalf("search %q matched %d items, want 1 literal match", "100% Cover", len(got))
	}
}

// TestCatalogSearchStillMatchesSubstrings guards against over-escaping: a normal
// substring query must still match via the surrounding wildcards.
func TestCatalogSearchStillMatchesSubstrings(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	tid := seedTenant(t, conn)
	vID := seedBareVersion(t, conn, tid)

	seedNamedItem(t, conn, tid, vID, "C1", "Self_Care")
	seedNamedItem(t, conn, tid, vID, "C2", "SelfXCare")

	got, err := repo.SearchItems(ctx, tid, vID, "Self")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("substring search %q matched %d items, want 2", "Self", len(got))
	}
}

// TestSearchItemsAllFieldsTenantScoped proves the all-fields (code/name/category/
// unit) search is tenant-scoped: two tenants each own a version with an item that
// differs only by category + unit. A category-substring search for tenant A
// returns ONLY tenant A's item (never tenant B's), and a unit-substring search
// matches too.
func TestSearchItemsAllFieldsTenantScoped(t *testing.T) {
	conn := newTestDB(t)
	repo := NewItems(conn)
	ctx := context.Background()
	q := gen.New(conn)

	tidA := seedTenant(t, conn)
	tidB := seedTenant(t, conn)
	vA := seedBareVersion(t, conn, tidA)
	vB := seedBareVersion(t, conn, tidB)

	// Both items share code/name; they differ only by category + unit.
	if _, err := q.CreateItem(ctx, gen.CreateItemParams{
		TenantID: tidA, Uuid: uuid.NewString(), PriceListVersionID: vA,
		Code: "AAA", Name: "Shared Name", Taxable: 0,
		Category: sql.NullString{String: "AlphaCat", Valid: true},
		Unit:     sql.NullString{String: "HourA", Valid: true},
	}); err != nil {
		t.Fatalf("CreateItem A: %v", err)
	}
	if _, err := q.CreateItem(ctx, gen.CreateItemParams{
		TenantID: tidB, Uuid: uuid.NewString(), PriceListVersionID: vB,
		Code: "BBB", Name: "Shared Name", Taxable: 0,
		Category: sql.NullString{String: "BetaCat", Valid: true},
		Unit:     sql.NullString{String: "HourB", Valid: true},
	}); err != nil {
		t.Fatalf("CreateItem B: %v", err)
	}

	// Category substring for tenant A: only A's item, never B's.
	byCat, err := repo.SearchItems(ctx, tidA, vA, "AlphaCat")
	if err != nil {
		t.Fatalf("SearchItems category: %v", err)
	}
	if len(byCat) != 1 || byCat[0].Code != "AAA" {
		t.Fatalf("category search = %+v, want exactly tenant A's AAA", byCat)
	}

	// Tenant A must not see B's category at all (proves scoping, not just the join).
	leak, err := repo.SearchItems(ctx, tidA, vA, "BetaCat")
	if err != nil {
		t.Fatalf("SearchItems leak probe: %v", err)
	}
	if len(leak) != 0 {
		t.Fatalf("tenant A leaked tenant B's item: %+v", leak)
	}

	// Unit substring also matches, scoped to tenant A.
	byUnit, err := repo.SearchItems(ctx, tidA, vA, "HourA")
	if err != nil {
		t.Fatalf("SearchItems unit: %v", err)
	}
	if len(byUnit) != 1 || byUnit[0].Code != "AAA" {
		t.Fatalf("unit search = %+v, want exactly tenant A's AAA", byUnit)
	}
}
