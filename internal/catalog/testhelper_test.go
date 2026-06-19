package catalog

import (
	"database/sql"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Migrations seed the real NDIS catalogue (00006); catalogue tests assert
	// against their own ingested versions, so start from a clean catalogue.
	if _, err := conn.Exec("DELETE FROM catalog_versions"); err != nil {
		t.Fatalf("clear catalogue: %v", err)
	}
	return conn
}
