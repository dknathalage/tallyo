package audit

import (
	"context"
	"database/sql"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

func TestLogInsertsRow(t *testing.T) {
	conn := mustDB(t)
	err := Log(context.Background(), conn, Entry{
		EntityType: "business_profile",
		EntityID:   ids.New(),
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
	tenantID, userID := seedTenantUser(t, conn)

	entityID := ids.New()
	ctx := reqctx.WithUser(reqctx.WithTenant(context.Background(), tenantID), userID)
	if err := Log(ctx, conn, Entry{EntityType: "invoice", EntityID: entityID, Action: "create"}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	var gotTenant, gotUser sql.NullString
	if err := conn.QueryRow(
		"SELECT tenant_id, user_id FROM audit_log WHERE entity_type='invoice' AND entity_id=$1", entityID,
	).Scan(&gotTenant, &gotUser); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !gotTenant.Valid || gotTenant.String != tenantID {
		t.Fatalf("tenant_id = %+v, want %s", gotTenant, tenantID)
	}
	if !gotUser.Valid || gotUser.String != userID {
		t.Fatalf("user_id = %+v, want %s", gotUser, userID)
	}
}

// TestLogNullStampsWhenNoContext verifies the global/system path: with no tenant
// or user on the context (e.g. catalogue ingest, sweeps), both columns are NULL.
func TestLogNullStampsWhenNoContext(t *testing.T) {
	conn := mustDB(t)
	entityID := ids.New()
	if err := Log(context.Background(), conn, Entry{EntityType: "catalog_version", EntityID: entityID, Action: "ingest"}); err != nil {
		t.Fatalf("Log: %v", err)
	}
	var gotTenant, gotUser sql.NullString
	if err := conn.QueryRow(
		"SELECT tenant_id, user_id FROM audit_log WHERE entity_type='catalog_version' AND entity_id=$1", entityID,
	).Scan(&gotTenant, &gotUser); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if gotTenant.Valid || gotUser.Valid {
		t.Fatalf("expected NULL tenant_id+user_id, got tenant=%+v user=%+v", gotTenant, gotUser)
	}
}

// seedTenantUser inserts a tenant and a user so audit FK constraints hold.
func seedTenantUser(t *testing.T, conn *sql.DB) (tenantID, userID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tenantID = ids.New()
	if _, err := conn.Exec(
		`INSERT INTO tenants (id, name, status, created_at, updated_at) VALUES ($1, 'Acme', 'active', $2, $3)`,
		tenantID, now, now); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	userID = ids.New()
	if _, err := conn.Exec(
		`INSERT INTO users (id, tenant_id, email, firebase_uid, name, role, created_at, updated_at)
		 VALUES ($1, $2, 'o@acme.test', 'uid-owner', 'Owner', 'owner', $3, $4)`,
		userID, tenantID, now, now); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return tenantID, userID
}

func mustDB(t *testing.T) *sql.DB {
	t.Helper()
	conn := appdb.OpenTestDB(t)
	return conn
}
