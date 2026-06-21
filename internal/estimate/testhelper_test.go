package estimate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/google/uuid"
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

// tctx returns a context carrying the given tenant id.
func tctx(tenantID int64) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}

// seedParticipant inserts a minimal participant for a tenant and returns its id.
func seedParticipant(t *testing.T, conn *sql.DB, tenantID int64, name string) int64 {
	t.Helper()
	p, err := participant.NewParticipants(conn).Create(context.Background(), tenantID, participant.ParticipantInput{Name: name})
	if err != nil {
		t.Fatalf("seedParticipant %q: %v", name, err)
	}
	return p.ID
}

// newEstimateSvc creates a migrated DB, seeds a tenant+participant, and returns
// the estimate Service, Hub, tenantID, participantID.
func newEstimateSvc(t *testing.T) (*Service, *realtime.Hub, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	participantID := seedParticipant(t, conn, tenantID, "Jane Participant")
	hub := realtime.NewHub()
	return NewService(conn, conn, hub), hub, tenantID, participantID
}

// makeEstimate creates a single estimate for the tenant/participant.
func makeEstimate(t *testing.T, svc *Service, tenantID, participantID int64) *Estimate {
	t.Helper()
	est, err := svc.Create(tctx(tenantID), EstimateInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", ValidUntil: "2026-02-01",
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
func mkEstimate(t *testing.T, repo *EstimatesRepo, tid, pid int64) *Estimate {
	t.Helper()
	est, err := repo.Create(context.Background(), tid, EstimateInput{
		ParticipantID: pid, IssueDate: "2026-01-01", ValidUntil: "2026-02-01", Tax: 10,
	}, []billing.LineItemInput{{Description: "Support", Quantity: 2, UnitPrice: 50}})
	if err != nil {
		t.Fatalf("Create estimate: %v", err)
	}
	return est
}
