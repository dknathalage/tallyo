package db

import (
	"database/sql"
	"testing"
)

// tablePresent reports whether a base table of the given name exists in the
// public schema, using the Postgres information_schema catalog (the replacement
// for SQLite's sqlite_master).
func tablePresent(t *testing.T, conn *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := conn.QueryRow(
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name = $1`, name,
	).Scan(&n); err != nil {
		t.Fatalf("query information_schema.tables for %s: %v", name, err)
	}
	return n > 0
}

// columnPresent reports whether table has the named column (information_schema
// replacing SQLite's PRAGMA table_info).
func columnPresent(t *testing.T, conn *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	if err := conn.QueryRow(
		`SELECT count(*) FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
		table, column,
	).Scan(&n); err != nil {
		t.Fatalf("query information_schema.columns for %s.%s: %v", table, column, err)
	}
	return n > 0
}

func TestMigrateCreatesTenancyTables(t *testing.T) {
	conn := OpenTestDB(t)
	for _, tbl := range []string{"tenants", "users", "invites", "business_profile"} {
		if !tablePresent(t, conn, tbl) {
			t.Fatalf("table %s missing", tbl)
		}
	}
	// The scs sessions table was dropped by the GCIP migration (stateless auth).
	if tablePresent(t, conn, "sessions") {
		t.Fatal("sessions table should be dropped by GCIP migration")
	}
	// users.password_hash dropped; users.firebase_uid added.
	if columnPresent(t, conn, "users", "password_hash") {
		t.Fatal("users.password_hash should be dropped")
	}
	if !columnPresent(t, conn, "users", "firebase_uid") {
		t.Fatal("users.firebase_uid should exist")
	}
}

func TestMigrateCreatesTenantBusinessTables(t *testing.T) {
	conn := OpenTestDB(t)
	for _, tbl := range []string{
		"payers", "clients", "catalogue_items", "tax_rates",
		"invoices", "line_items", "estimates", "estimate_line_items",
		"payments", "audit_log",
	} {
		if !tablePresent(t, conn, tbl) {
			t.Fatalf("table %s missing", tbl)
		}
	}
}

func TestMigrateCreatesCatalogueTable(t *testing.T) {
	conn := OpenTestDB(t)
	// The merged catalogue table is present.
	if !tablePresent(t, conn, "catalogue_items") {
		t.Fatal("table catalogue_items missing")
	}
	// The old catalogue tables are gone.
	for _, tbl := range []string{"custom_items", "price_list_versions", "items"} {
		if tablePresent(t, conn, tbl) {
			t.Fatalf("table %s should be absent after merge", tbl)
		}
	}
	// line_items collapsed the three catalogue refs into one catalogue_item_id.
	if !columnPresent(t, conn, "line_items", "catalogue_item_id") {
		t.Fatal("line_items must have catalogue_item_id column")
	}
	for _, old := range []string{"item_id", "custom_item_id", "price_list_version_id"} {
		if columnPresent(t, conn, "line_items", old) {
			t.Fatalf("line_items must not have %s column after merge", old)
		}
	}
}

func TestMigrateRemovesLegacyTablesAndColumns(t *testing.T) {
	conn := OpenTestDB(t)
	// Removed tables must be absent.
	for _, tbl := range []string{"rate_tiers", "catalog_item_rates", "catalog_items", "column_mappings"} {
		if tablePresent(t, conn, tbl) {
			t.Fatalf("table %s should be absent", tbl)
		}
	}
	// clients must not carry the old pricing_tier_id column.
	if columnPresent(t, conn, "clients", "pricing_tier_id") {
		t.Fatal("clients must not have pricing_tier_id column")
	}
}

func TestMigrateEnforcesTenantUniqueConstraints(t *testing.T) {
	conn := OpenTestDB(t)
	now := "2026-06-16T00:00:00Z"
	if _, err := conn.Exec(
		"INSERT INTO tenants (id, name, status, created_at, updated_at) VALUES ($1,$2,$3,$4,$5)",
		"t1", "Tenant One", "active", now, now,
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	// UNIQUE(tenant_id, email) on users: same email in same tenant must fail.
	if _, err := conn.Exec(
		"INSERT INTO users (id, tenant_id, email, firebase_uid, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)",
		"u1", "t1", "a@b.com", "uid1", now, now,
	); err != nil {
		t.Fatalf("insert first user: %v", err)
	}
	if _, err := conn.Exec(
		"INSERT INTO users (id, tenant_id, email, firebase_uid, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)",
		"u2", "t1", "a@b.com", "uid2", now, now,
	); err == nil {
		t.Fatalf("expected UNIQUE(tenant_id, email) violation, got nil")
	}
	// UNIQUE(tenant_id, firebase_uid) on users: same uid in same tenant must fail.
	if _, err := conn.Exec(
		"INSERT INTO users (id, tenant_id, email, firebase_uid, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)",
		"u1b", "t1", "diff@b.com", "uid1", now, now,
	); err == nil {
		t.Fatalf("expected UNIQUE(tenant_id, firebase_uid) violation, got nil")
	}
	// FK enforcement: inserting a user for a non-existent tenant must fail.
	if _, err := conn.Exec(
		"INSERT INTO users (id, tenant_id, email, firebase_uid, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)",
		"u3", "no-such-tenant", "c@d.com", "uid3", now, now,
	); err == nil {
		t.Fatalf("expected FK violation for missing tenant, got nil")
	}
}

func TestMigrateEnforcesEnumChecks(t *testing.T) {
	conn := OpenTestDB(t)
	now := "2026-06-16T00:00:00Z"
	// Invalid tenants.status must be rejected by CHECK.
	if _, err := conn.Exec(
		"INSERT INTO tenants (id, name, status, created_at, updated_at) VALUES ($1,$2,$3,$4,$5)",
		"tbad", "Bad", "frozen", now, now,
	); err == nil {
		t.Fatalf("expected CHECK violation for tenants.status='frozen', got nil")
	}
}

func TestMigrateForeignKeyDeleteBehavior(t *testing.T) {
	conn := OpenTestDB(t)
	now := "2026-06-16T00:00:00Z"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec("INSERT INTO tenants (id, name, status, created_at, updated_at) VALUES ($1,$2,$3,$4,$5)",
		"t1", "Tenant", "active", now, now)
	exec("INSERT INTO payers (id, tenant_id, name, created_at, updated_at) VALUES ($1,$2,$3,$4,$5)",
		"pm1", "t1", "PM", now, now)
	exec("INSERT INTO clients (id, tenant_id, name, payer_id, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)",
		"p1", "t1", "Pat", "pm1", now, now)
	exec("INSERT INTO invoices (id, tenant_id, number, client_id, payer_id, issue_date, due_date, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)",
		"inv1", "t1", "INV-1", "p1", "pm1", now, now, now, now)
	exec("INSERT INTO line_items (id, tenant_id, invoice_id, description) VALUES ($1,$2,$3,$4)",
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
	exec("INSERT INTO invoices (id, tenant_id, number, client_id, payer_id, issue_date, due_date, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)",
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
	conn := OpenTestDB(t)
	// OpenTestDB already migrated; a second Migrate must be a no-op.
	if err := Migrate(conn); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}
