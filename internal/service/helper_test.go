package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/repository"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/google/uuid"
)

// newTestDB opens a fresh migrated in-temp SQLite DB for a service test.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "svc.db"))
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
// tenants(id) via FK, so every service test must seed at least one tenant.
// (Replicated here because the repository package's test helper is not
// importable from this test package.)
func seedTenant(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		Uuid:      uuid.NewString(),
		Name:      "Acme NDIS",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedTenant: %v", err)
	}
	return tn.ID
}

// tctx returns a context carrying the given tenant id. Every tenant-scoped
// service method calls reqctx.MustTenant and panics without it.
func tctx(tenantID int64) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}

// seedParticipant inserts a minimal participant for a tenant and returns its id.
// Used by invoice/estimate/payment/recurring tests that need a valid FK.
func seedParticipant(t *testing.T, conn *sql.DB, tenantID int64) int64 {
	t.Helper()
	p, err := repository.NewParticipants(conn).Create(tctx(tenantID), tenantID, repository.ParticipantInput{Name: "Jane Participant"})
	if err != nil {
		t.Fatalf("seedParticipant: %v", err)
	}
	return p.ID
}
