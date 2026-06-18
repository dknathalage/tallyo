package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/google/uuid"
)

// newTestDB opens a fresh migrated in-temp SQLite DB for a test.
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
	return conn
}

// seedTenant creates a tenant and returns its id. Tenant-owned rows reference
// tenants(id) via FK, so every repository test must seed at least one tenant.
func seedTenant(t *testing.T, conn *sql.DB, name string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		Uuid:      uuid.NewString(),
		Name:      name,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedTenant %q: %v", name, err)
	}
	return tn.ID
}

// seedParticipant inserts a minimal participant for a tenant and returns its id.
// Used by invoice/estimate/payment tests that need a valid participant FK.
func seedParticipant(t *testing.T, conn *sql.DB, tenantID int64, name string) int64 {
	t.Helper()
	p, err := participant.NewParticipants(conn).Create(context.Background(), tenantID, participant.ParticipantInput{Name: name})
	if err != nil {
		t.Fatalf("seedParticipant %q: %v", name, err)
	}
	return p.ID
}

// seedUser inserts a member user for the tenant and returns its id.
func seedUser(t *testing.T, conn *sql.DB, tenantID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	u, err := gen.New(conn).CreateUser(context.Background(), gen.CreateUserParams{
		Uuid: uuid.NewString(), TenantID: tenantID, Email: uuid.NewString() + "@x.com",
		PasswordHash: "x", Name: "U", Role: "member", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	return u.ID
}
