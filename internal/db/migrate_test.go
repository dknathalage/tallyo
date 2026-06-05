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

func TestMigrateCreatesAuthTables(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, tbl := range []string{"users", "invites", "sessions"} {
		var n string
		if err := conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}

func TestMigrateCreatesRateTiersPayers(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "rt.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, tbl := range []string{"rate_tiers", "payers"} {
		var n string
		if err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}

func TestMigrateCreatesBatch2Tables(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "b2.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, tbl := range []string{"tax_rates", "clients", "catalog_items", "catalog_item_rates"} {
		var n string
		if err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}

func TestMigrateCreatesInvoiceTables(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "inv.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, tbl := range []string{"invoices", "line_items"} {
		var n string
		if err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
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
