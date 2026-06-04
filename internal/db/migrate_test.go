package db

import (
	"path/filepath"
	"testing"
)

func TestMigrateCreatesTables(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, table := range []string{"audit_log", "business_profile"} {
		var name string
		err := conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(conn); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}
