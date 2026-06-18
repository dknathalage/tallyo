package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
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

// seedSuspendedTenant creates a tenant and marks it suspended, returning its id.
// Used by the per-tenant sweep test to assert suspended tenants are skipped.
func seedSuspendedTenant(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	id := seedTenant(t, conn)
	now := time.Now().UTC().Format(time.RFC3339)
	if err := gen.New(conn).UpdateTenantStatus(context.Background(), gen.UpdateTenantStatusParams{
		Status: "suspended", UpdatedAt: now, ID: id,
	}); err != nil {
		t.Fatalf("suspend tenant: %v", err)
	}
	return id
}

// seedSentPastDue creates an invoice, flips it to 'sent', and back-dates its
// due_date into the past so the overdue sweep selects it. Returns the invoice.
func seedSentPastDue(t *testing.T, conn *sql.DB, svc *InvoiceService, tenantID, participantID int64) *repository.Invoice {
	t.Helper()
	ctx := tctx(tenantID)
	inv, err := svc.Create(ctx, repository.InvoiceInput{
		ParticipantID: participantID, IssueDate: "2026-01-01", DueDate: "2026-01-15",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 5}})
	if err != nil {
		t.Fatalf("seedSentPastDue create: %v", err)
	}
	past := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")
	if _, err := conn.Exec(
		`UPDATE invoices SET status='sent', due_date=? WHERE tenant_id=? AND id=?`,
		past, tenantID, inv.ID); err != nil {
		t.Fatalf("seedSentPastDue backdate: %v", err)
	}
	return inv
}

// containsID reports whether ids contains target.
func containsID(ids []int64, target int64) bool {
	for i := range ids { // bounded by len(ids)
		if ids[i] == target {
			return true
		}
	}
	return false
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

// seedDraftInvoice inserts a minimal draft invoice and returns its id. Used by
// the shift-linkage tests that need an existing invoice to mark shifts drafted.
func seedDraftInvoice(t *testing.T, conn *sql.DB, tenantID, participantID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	inv, err := gen.New(conn).CreateInvoice(context.Background(), gen.CreateInvoiceParams{
		Uuid: uuid.NewString(), TenantID: tenantID, Number: uuid.NewString(), ParticipantID: participantID,
		Status: "draft", IssueDate: "2026-01-01", DueDate: "2026-02-01", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedDraftInvoice: %v", err)
	}
	return inv.ID
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
