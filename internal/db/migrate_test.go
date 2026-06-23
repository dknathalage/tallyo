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
		"plan_managers", "clients", "custom_items", "tax_rates",
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

func TestMigrateCreatesGlobalCatalogTables(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, tbl := range []string{"catalog_versions", "support_items", "support_item_prices"} {
		var n string
		if err := conn.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&n); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
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
	for _, tbl := range []string{"rate_tiers", "catalog_item_rates", "catalog_items", "payers", "column_mappings"} {
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
		"INSERT INTO tenants (uuid, name, status, created_at, updated_at) VALUES (?,?,?,?,?)",
		"t1", "Tenant One", "active", now, now,
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	// UNIQUE(tenant_id, email) on users: same email in same tenant must fail.
	if _, err := conn.Exec(
		"INSERT INTO users (uuid, tenant_id, email, password_hash, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"u1", 1, "a@b.com", "h", now, now,
	); err != nil {
		t.Fatalf("insert first user: %v", err)
	}
	if _, err := conn.Exec(
		"INSERT INTO users (uuid, tenant_id, email, password_hash, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"u2", 1, "a@b.com", "h", now, now,
	); err == nil {
		t.Fatalf("expected UNIQUE(tenant_id, email) violation, got nil")
	}
	// FK enforcement: inserting a user for a non-existent tenant must fail.
	if _, err := conn.Exec(
		"INSERT INTO users (uuid, tenant_id, email, password_hash, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"u3", 999, "c@d.com", "h", now, now,
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
		"INSERT INTO tenants (uuid, name, status, created_at, updated_at) VALUES (?,?,?,?,?)",
		"tbad", "Bad", "frozen", now, now,
	); err == nil {
		t.Fatalf("expected CHECK violation for tenants.status='frozen', got nil")
	}
	// Valid tenant for downstream enum checks.
	if _, err := conn.Exec(
		"INSERT INTO tenants (uuid, name, status, created_at, updated_at) VALUES (?,?,?,?,?)",
		"t1", "Tenant One", "active", now, now,
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	// Invalid business_profile.zone must be rejected (load-bearing for price caps).
	if _, err := conn.Exec(
		"INSERT INTO business_profile (uuid, tenant_id, name, zone, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"bp1", 1, "Biz", "metro", now, now,
	); err == nil {
		t.Fatalf("expected CHECK violation for business_profile.zone='metro', got nil")
	}
	// Invalid clients.mgmt_type must be rejected.
	if _, err := conn.Exec(
		"INSERT INTO clients (uuid, tenant_id, name, mgmt_type, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		"p1", 1, "Pat", "agency", now, now,
	); err == nil {
		t.Fatalf("expected CHECK violation for clients.mgmt_type='agency', got nil")
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
	exec("INSERT INTO tenants (uuid, name, status, created_at, updated_at) VALUES (?,?,?,?,?)",
		"t1", "Tenant", "active", now, now)
	exec("INSERT INTO plan_managers (uuid, tenant_id, name, created_at, updated_at) VALUES (?,?,?,?,?)",
		"pm1", 1, "PM", now, now)
	exec("INSERT INTO clients (uuid, tenant_id, name, mgmt_type, plan_manager_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?)",
		"p1", 1, "Pat", "plan", 1, now, now)
	exec("INSERT INTO invoices (uuid, tenant_id, number, client_id, plan_manager_id, issue_date, due_date, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
		"inv1", 1, "INV-1", 1, 1, now, now, now, now)
	exec("INSERT INTO line_items (uuid, tenant_id, invoice_id, description) VALUES (?,?,?,?)",
		"li1", 1, 1, "a service")

	// CASCADE: deleting the invoice removes its line_items.
	exec("DELETE FROM invoices WHERE id = 1")
	var liCount int
	if err := conn.QueryRow("SELECT COUNT(*) FROM line_items WHERE invoice_id = 1").Scan(&liCount); err != nil {
		t.Fatalf("count line_items: %v", err)
	}
	if liCount != 0 {
		t.Fatalf("expected line_items cascade-deleted, got count=%d", liCount)
	}

	// SET NULL: a new invoice referencing the plan_manager loses the ref on delete.
	exec("INSERT INTO invoices (uuid, tenant_id, number, client_id, plan_manager_id, issue_date, due_date, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
		"inv2", 1, "INV-2", 1, 1, now, now, now, now)
	exec("DELETE FROM plan_managers WHERE id = 1")
	var pmID *int64
	if err := conn.QueryRow("SELECT plan_manager_id FROM invoices WHERE uuid = 'inv2'").Scan(&pmID); err != nil {
		t.Fatalf("read invoice plan_manager_id: %v", err)
	}
	if pmID != nil {
		t.Fatalf("expected invoice.plan_manager_id set NULL on plan_manager delete, got %d", *pmID)
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
