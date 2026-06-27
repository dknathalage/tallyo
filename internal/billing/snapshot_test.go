package billing_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/ids"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"

	"github.com/dknathalage/tallyo/internal/billing"
)

// newTestDB opens a fresh migrated in-temp SQLite DB for a test.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn := appdb.OpenTestDB(t)
	return conn
}

// seedTenant creates a tenant and returns its id.
func seedTenant(t *testing.T, conn *sql.DB, name string) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		ID:        ids.New(),
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

func TestSnapshotBuilderBusiness(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, conn, "Snap Test Org")
	now := time.Now().UTC().Format(time.RFC3339)

	// Seed a business profile for the tenant.
	if err := gen.New(conn).UpsertBusinessProfile(ctx, gen.UpsertBusinessProfileParams{
		TenantID:  tenantID,
		ID:        ids.New(),
		Name:      "Snap Co",
		Email:     sql.NullString{String: "snap@example.com", Valid: true},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertBusinessProfile: %v", err)
	}

	sb := billing.NewSnapshotBuilder(conn)
	got := sb.Business(ctx, tenantID)

	if got == "" || got == "{}" {
		t.Fatalf("Business() = %q, want non-empty JSON with name", got)
	}
	if !strings.Contains(got, "Snap Co") {
		t.Fatalf("Business() = %q, want to contain %q", got, "Snap Co")
	}
}

func TestSnapshotBuilderBusinessMissing(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, conn, "Empty Org")

	sb := billing.NewSnapshotBuilder(conn)
	got := sb.Business(ctx, tenantID)
	if got != "{}" {
		t.Fatalf("Business() on missing profile = %q, want {}", got)
	}
}

func TestSnapshotBuilderPayerNil(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, conn, "T")

	sb := billing.NewSnapshotBuilder(conn)
	got := sb.Payer(ctx, tenantID, nil)
	if got != "{}" {
		t.Fatalf("Payer(nil) = %q, want {}", got)
	}
}

func TestNewSnapshotBuilderPanicsOnNilDB(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewSnapshotBuilder(nil) did not panic")
		}
	}()
	billing.NewSnapshotBuilder(nil)
}
