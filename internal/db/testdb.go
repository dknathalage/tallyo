package db

import (
	"database/sql"
	"os"
	"strings"
	"testing"
)

// OpenTestDB opens the shared Postgres test database named by TEST_DATABASE_URL,
// applies migrations, then truncates every business + control table (leaving the
// goose version tables intact) so each test starts from a clean slate. When the
// env var is unset the test is skipped — DB-backed tests do not run without a
// real Postgres.
//
// All test packages share ONE Postgres database, so the suite must run with
// `-p 1` (serialized packages); the truncate-on-open here gives each test a clean
// schema without dropping the version-tracked migrations.
func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres-backed test")
	}
	conn, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(conn); err != nil {
		_ = conn.Close()
		t.Fatalf("Migrate: %v", err)
	}
	if err := truncateAll(conn); err != nil {
		_ = conn.Close()
		t.Fatalf("truncate: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// truncateAll empties every public table except the goose version tables. It
// gathers the table list from information_schema then issues one TRUNCATE ...
// RESTART IDENTITY CASCADE so FK order does not matter.
func truncateAll(conn *sql.DB) error {
	rows, err := conn.Query(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		  AND table_name NOT IN ('goose_db_version', 'goose_tenant_version')`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var tables []string
	for rows.Next() { // bounded by the fixed table count
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, `"`+name+`"`)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}
	_, err = conn.Exec("TRUNCATE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE")
	return err
}
