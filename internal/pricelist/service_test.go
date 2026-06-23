package pricelist

import "testing"

// With no catalogue imported, ListVersions returns an empty (non-nil) slice.
func TestSupportCatalogListVersionsEmpty(t *testing.T) {
	conn := newTestDB(t)
	svc := NewService(conn)

	versions, err := svc.ListVersions(tctx(seedTenant(t, conn)))
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("ListVersions = %d, want 0", len(versions))
	}
}
