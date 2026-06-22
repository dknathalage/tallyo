package recurring

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

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
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "recurring.db"))
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

// seedParticipant inserts a minimal participant for a tenant and returns its uuid
// (the public identifier; recurring templates reference participants by uuid).
func seedParticipant(t *testing.T, conn *sql.DB, tenantID int64, name string) string {
	t.Helper()
	p, err := participant.NewParticipants(conn).Create(context.Background(), tenantID, participant.ParticipantInput{Name: name})
	if err != nil {
		t.Fatalf("seedParticipant %q: %v", name, err)
	}
	return p.UUID
}

// newRecurringSvc creates a migrated DB, seeds a tenant+participant, and returns
// the recurring Service, Hub, tenantID, participant uuid.
func newRecurringSvc(t *testing.T) (*Service, *realtime.Hub, int64, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	participantUUID := seedParticipant(t, conn, tenantID, "Jane Participant")
	hub := realtime.NewHub()
	return NewService(conn, hub), hub, tenantID, participantUUID
}

// mkTemplate creates a recurring template via the repo, referencing the
// participant by uuid.
func mkTemplate(t *testing.T, repo *Repo, tid int64, pUUID, nextDue string) *RecurringTemplate {
	t.Helper()
	pid := pUUID
	tpl, err := repo.Create(context.Background(), tid, RecurringInput{
		ParticipantUUID: &pid,
		Name:            "Weekly support",
		Frequency:       "weekly",
		NextDue:         nextDue,
		TaxRate:         10,
		LineItems:       []RecurringLine{{Description: "Support", Quantity: 1, UnitPrice: 100}},
		IsActive:        true,
	})
	if err != nil {
		t.Fatalf("Create template: %v", err)
	}
	return tpl
}

// seedRecurringInput builds a valid monthly template input for the given
// participant uuid, due in the past so GenerateOne will produce an invoice.
func seedRecurringInput(participantUUID string) RecurringInput {
	pid := participantUUID
	return RecurringInput{
		ParticipantUUID: &pid,
		Name:            "Monthly",
		Frequency:       "monthly",
		NextDue:         "2026-01-01",
		LineItems: []RecurringLine{
			{Description: "A", Quantity: 2, UnitPrice: 10, SortOrder: 0},
			{Description: "B", Quantity: 1, UnitPrice: 5, SortOrder: 1},
		},
		TaxRate:  10,
		IsActive: true,
	}
}
