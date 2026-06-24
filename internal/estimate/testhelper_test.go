package estimate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/client"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

// newTestDB opens a fresh migrated in-temp SQLite DB.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "estimate.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
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

// tctx returns a context carrying the given tenant id.
func tctx(tenantID string) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}

// seedClient inserts a minimal client for a tenant and returns its id.
func seedClient(t *testing.T, conn *sql.DB, tenantID string, name string) string {
	t.Helper()
	p, err := client.NewClients(conn).Create(context.Background(), tenantID, client.ClientInput{Name: name})
	if err != nil {
		t.Fatalf("seedClient %q: %v", name, err)
	}
	return p.ID
}

// newEstimateSvc creates a migrated DB, seeds a tenant+client, and returns
// the estimate Service, Hub, tenantID, clientID.
func newEstimateSvc(t *testing.T) (*Service, *realtime.Hub, string, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	hub := realtime.NewHub()
	return NewService(conn, hub), hub, tenantID, clientID
}

// makeEstimate creates a single estimate for the tenant/client.
func makeEstimate(t *testing.T, svc *Service, tenantID, clientID string) *Estimate {
	t.Helper()
	est, err := svc.Create(tctx(tenantID), EstimateInput{
		ClientID: clientID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 10}})
	if err != nil {
		t.Fatalf("makeEstimate: %v", err)
	}
	if est == nil {
		t.Fatal("makeEstimate: nil estimate")
	}
	return est
}

// mkEstimate creates an estimate via repo directly.
func mkEstimate(t *testing.T, repo *EstimatesRepo, tid, pid string) *Estimate {
	t.Helper()
	est, err := repo.Create(context.Background(), tid, EstimateInput{
		ClientID: pid, IssueDate: "2026-01-01", ValidUntil: "2026-02-01", Tax: 10,
	}, []billing.LineItemInput{{Description: "Support", Quantity: 2, UnitPrice: 50}})
	if err != nil {
		t.Fatalf("Create estimate: %v", err)
	}
	return est
}
