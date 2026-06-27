package billing

// testhelper_test.go — minimal DB/tenant/context helpers for the validation
// tests (package billing, internal). These are distinct from snapshot_test.go
// which lives in the external package billing_test.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// newTestDB opens the shared migrated Postgres test DB for a billing test.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn := appdb.OpenTestDB(t)
	// A freshly migrated catalogue is empty; tests seed their own items.
	return conn
}

// seedTenant creates a tenant and returns its id. Tenant-owned rows reference
// tenants(id) via FK, so every validation test must seed at least one tenant.
func seedTenant(t *testing.T, conn *sql.DB) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		ID:        ids.New(),
		Name:      "Acme",
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
func tctx(tenantID string) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}
