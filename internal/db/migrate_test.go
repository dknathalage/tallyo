package db

import (
	"path/filepath"
	"testing"
)

func TestMigrateCreatesTenancyTables(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "tenancy.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, tbl := range []string{"tenants", "users", "invites", "sessions", "business_profile"} {
		var n string
		if err := conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}

func TestMigrateCreatesTenantBusinessTables(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "biz.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, tbl := range []string{
		"payers", "clients", "catalogue_items", "tax_rates",
		"invoices", "line_items", "estimates", "estimate_line_items",
		"payments", "recurring_templates", "audit_log",
	} {
		var n string
		if err := conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}

func TestMigrateCreatesCatalogueTable(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// The merged catalogue table is present.
	var n string
	if err := conn.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?", "catalogue_items",
	).Scan(&n); err != nil {
		t.Fatalf("table catalogue_items missing: %v", err)
	}
	// The old catalogue tables are gone.
	for _, tbl := range []string{"custom_items", "price_list_versions", "items"} {
		var count int
		if err := conn.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&count); err != nil {
			t.Fatalf("query sqlite_master for %s: %v", tbl, err)
		}
		if count != 0 {
			t.Fatalf("table %s should be absent after merge, got count=%d", tbl, count)
		}
	}
	// line_items collapsed the three catalogue refs into one catalogue_item_id.
	cols := map[string]bool{}
	rows, err := conn.Query("PRAGMA table_info(line_items)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var colName, colType string
		var dflt any
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		cols[colName] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if !cols["catalogue_item_id"] {
		t.Fatalf("line_items must have catalogue_item_id column")
	}
	for _, old := range []string{"item_id", "custom_item_id", "price_list_version_id"} {
		if cols[old] {
			t.Fatalf("line_items must not have %s column after merge", old)
		}
	}
}

func TestMigrateRemovesLegacyTablesAndColumns(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Removed tables must be absent.
	for _, tbl := range []string{"rate_tiers", "catalog_item_rates", "catalog_items", "column_mappings"} {
		var count int
		if err := conn.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&count); err != nil {
			t.Fatalf("query sqlite_master for %s: %v", tbl, err)
		}
		if count != 0 {
			t.Fatalf("table %s should be absent, got count=%d", tbl, count)
		}
	}
	// clients must not carry the old pricing_tier_id column.
	rows, err := conn.Query("PRAGMA table_info(clients)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var colName, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if colName == "pricing_tier_id" {
			t.Fatalf("clients must not have pricing_tier_id column")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
}

func TestMigrateEnforcesTenantUniqueConstraints(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "uniq.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := "2026-06-16T00:00:00Z"
	if _, err := conn.Exec(
		"INSERT INTO tenants (id, name, status, created_at, updated_at) VALUES (?,?,?,?,?)",
		"t1", "Tenant One", "active", now, now,
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	// UNIQUE(tenant_id, email) on users: same email in same tenant must fail.
	if _, err := conn.Exec(
		"INSERT INTO users (id, tenant_id, email, password_hash, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"u1", "t1", "a@b.com", "h", now, now,
	); err != nil {
		t.Fatalf("insert first user: %v", err)
	}
	if _, err := conn.Exec(
		"INSERT INTO users (id, tenant_id, email, password_hash, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"u2", "t1", "a@b.com", "h", now, now,
	); err == nil {
		t.Fatalf("expected UNIQUE(tenant_id, email) violation, got nil")
	}
	// FK enforcement: inserting a user for a non-existent tenant must fail.
	if _, err := conn.Exec(
		"INSERT INTO users (id, tenant_id, email, password_hash, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"u3", "no-such-tenant", "c@d.com", "h", now, now,
	); err == nil {
		t.Fatalf("expected FK violation for missing tenant, got nil")
	}
}

func TestMigrateEnforcesEnumChecks(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "checks.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := "2026-06-16T00:00:00Z"
	// Invalid tenants.status must be rejected by CHECK.
	if _, err := conn.Exec(
		"INSERT INTO tenants (id, name, status, created_at, updated_at) VALUES (?,?,?,?,?)",
		"tbad", "Bad", "frozen", now, now,
	); err == nil {
		t.Fatalf("expected CHECK violation for tenants.status='frozen', got nil")
	}
}

func TestMigrateForeignKeyDeleteBehavior(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "fkdel.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := "2026-06-16T00:00:00Z"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec("INSERT INTO tenants (id, name, status, created_at, updated_at) VALUES (?,?,?,?,?)",
		"t1", "Tenant", "active", now, now)
	exec("INSERT INTO payers (id, tenant_id, name, created_at, updated_at) VALUES (?,?,?,?,?)",
		"pm1", "t1", "PM", now, now)
	exec("INSERT INTO clients (id, tenant_id, name, payer_id, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"p1", "t1", "Pat", "pm1", now, now)
	exec("INSERT INTO invoices (id, tenant_id, number, client_id, payer_id, issue_date, due_date, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
		"inv1", "t1", "INV-1", "p1", "pm1", now, now, now, now)
	exec("INSERT INTO line_items (id, tenant_id, invoice_id, description) VALUES (?,?,?,?)",
		"li1", "t1", "inv1", "a service")

	// CASCADE: deleting the invoice removes its line_items.
	exec("DELETE FROM invoices WHERE id = 'inv1'")
	var liCount int
	if err := conn.QueryRow("SELECT COUNT(*) FROM line_items WHERE invoice_id = 'inv1'").Scan(&liCount); err != nil {
		t.Fatalf("count line_items: %v", err)
	}
	if liCount != 0 {
		t.Fatalf("expected line_items cascade-deleted, got count=%d", liCount)
	}

	// SET NULL: a new invoice referencing the payer loses the ref on delete.
	exec("INSERT INTO invoices (id, tenant_id, number, client_id, payer_id, issue_date, due_date, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
		"inv2", "t1", "INV-2", "p1", "pm1", now, now, now, now)
	exec("DELETE FROM payers WHERE id = 'pm1'")
	var pmID *string
	if err := conn.QueryRow("SELECT payer_id FROM invoices WHERE id = 'inv2'").Scan(&pmID); err != nil {
		t.Fatalf("read invoice payer_id: %v", err)
	}
	if pmID != nil {
		t.Fatalf("expected invoice.payer_id set NULL on payer delete, got %s", *pmID)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "idem.db"))
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
