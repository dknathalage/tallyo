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
func seedNamedItem(t *testing.T, conn *sql.DB, versionID int64, code, name string) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	if _, err := q.CreateItem(ctx, gen.CreateItemParams{
		Uuid:               uuid.NewString(),
		PriceListVersionID: versionID,
		Code:               code,
		Name:               name,
		Taxable:            0,
	}); err != nil {
		t.Fatalf("CreateItem %q: %v", name, err)
	}
}

func seedBareVersion(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	v, err := gen.New(conn).CreatePriceListVersion(ctx, gen.CreatePriceListVersionParams{
		Uuid: uuid.NewString(), Label: "v", EffectiveFrom: "2025-07-01", CreatedAt: now,
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
	vID := seedBareVersion(t, conn)

	seedNamedItem(t, conn, vID, "CODE_LITERAL", "Self_Care")
	seedNamedItem(t, conn, vID, "CODE_WILDCARD", "SelfXCare")

	got, err := repo.SearchItems(ctx, vID, "Self_Care")
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
	vID := seedBareVersion(t, conn)

	seedNamedItem(t, conn, vID, "C1", "100% Cover")
	seedNamedItem(t, conn, vID, "C2", "100 then Cover")

	got, err := repo.SearchItems(ctx, vID, "100% Cover")
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
	vID := seedBareVersion(t, conn)

	seedNamedItem(t, conn, vID, "C1", "Self_Care")
	seedNamedItem(t, conn, vID, "C2", "SelfXCare")

	got, err := repo.SearchItems(ctx, vID, "Self")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("substring search %q matched %d items, want 2", "Self", len(got))
	}
}
