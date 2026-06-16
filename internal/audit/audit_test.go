package audit

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/google/uuid"
)

func TestLogInsertsRow(t *testing.T) {
	conn := mustDB(t)
	defer conn.Close()

	err := Log(context.Background(), conn, Entry{
		EntityType: "business_profile",
		EntityID:   1,
		Action:     "update",
		Changes:    `{"name":"Acme"}`,
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	var count int
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='business_profile' AND action='update'",
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit rows = %d, want 1", count)
	}
}

func TestLogValidatesInputs(t *testing.T) {
	conn := mustDB(t)
	defer conn.Close()

	cases := []struct {
		name  string
		entry Entry
	}{
		{
			name:  "empty entity_type",
			entry: Entry{EntityType: "", Action: "update"},
		},
		{
			name:  "empty action",
			entry: Entry{EntityType: "business_profile", Action: ""},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Log(context.Background(), conn, tc.entry); err == nil {
				t.Fatalf("Log(%+v): expected error, got nil", tc.entry)
			}
		})
	}
}

// TestLogStampsTenantAndUserFromContext verifies every audit row records the
// acting tenant_id and user_id sourced from reqctx.
func TestLogStampsTenantAndUserFromContext(t *testing.T) {
	conn := mustDB(t)
	defer conn.Close()
	tenantID, userID := seedTenantUser(t, conn)

	ctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)
	if err := Log(ctx, conn, Entry{EntityType: "invoice", EntityID: 5, Action: "create"}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	var gotTenant, gotUser sql.NullInt64
	if err := conn.QueryRow(
		"SELECT tenant_id, user_id FROM audit_log WHERE entity_type='invoice' AND entity_id=5",
	).Scan(&gotTenant, &gotUser); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !gotTenant.Valid || gotTenant.Int64 != tenantID {
		t.Fatalf("tenant_id = %+v, want %d", gotTenant, tenantID)
	}
	if !gotUser.Valid || gotUser.Int64 != userID {
		t.Fatalf("user_id = %+v, want %d", gotUser, userID)
	}
}

// TestLogNullStampsWhenNoContext verifies the global/system path: with no tenant
// or user on the context (e.g. catalogue ingest, sweeps), both columns are NULL.
func TestLogNullStampsWhenNoContext(t *testing.T) {
	conn := mustDB(t)
	defer conn.Close()

	if err := Log(context.Background(), conn, Entry{EntityType: "catalog_version", EntityID: 9, Action: "ingest"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	var gotTenant, gotUser sql.NullInt64
	if err := conn.QueryRow(
		"SELECT tenant_id, user_id FROM audit_log WHERE entity_type='catalog_version' AND entity_id=9",
	).Scan(&gotTenant, &gotUser); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if gotTenant.Valid || gotUser.Valid {
		t.Fatalf("expected NULL tenant_id+user_id, got tenant=%+v user=%+v", gotTenant, gotUser)
	}
}

// seedTenantUser inserts a tenant and a user so audit FK constraints hold.
func seedTenantUser(t *testing.T, conn *sql.DB) (tenantID, userID int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := conn.Exec(
		`INSERT INTO tenants (uuid, name, status, created_at, updated_at) VALUES (?, 'Acme', 'active', ?, ?)`,
		uuid.NewString(), now, now)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	tenantID, _ = res.LastInsertId()
	res, err = conn.Exec(
		`INSERT INTO users (uuid, tenant_id, email, password_hash, name, role, created_at, updated_at)
		 VALUES (?, ?, 'o@acme.test', 'x', 'Owner', 'owner', ?, ?)`,
		uuid.NewString(), tenantID, now, now)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	userID, _ = res.LastInsertId()
	return tenantID, userID
}

func mustDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}
