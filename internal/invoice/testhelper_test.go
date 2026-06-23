package invoice

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
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/session"
	"github.com/google/uuid"
)

// newTestDB opens a fresh migrated in-temp SQLite DB.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "invoice.db"))
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

// seedSuspendedTenant creates a tenant and marks it suspended.
func seedSuspendedTenant(t *testing.T, conn *sql.DB) int64 {
	t.Helper()
	id := seedTenant(t, conn, "Suspended Tenant")
	now := time.Now().UTC().Format(time.RFC3339)
	if err := gen.New(conn).UpdateTenantStatus(context.Background(), gen.UpdateTenantStatusParams{
		Status: "suspended", UpdatedAt: now, ID: id,
	}); err != nil {
		t.Fatalf("suspend tenant: %v", err)
	}
	return id
}

// tctx returns a context carrying the given tenant id.
func tctx(tenantID int64) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}

// seedClient inserts a minimal client for a tenant and returns its id.
func seedClient(t *testing.T, conn *sql.DB, tenantID int64, name string) int64 {
	t.Helper()
	id, _ := seedClientUUID(t, conn, tenantID, name)
	return id
}

// seedClientUUID seeds a client and returns both its int PK and uuid.
// The uuid is the public identifier the client-stats route now resolves.
func seedClientUUID(t *testing.T, conn *sql.DB, tenantID int64, name string) (int64, string) {
	t.Helper()
	p, err := client.NewClients(conn).Create(context.Background(), tenantID, client.ClientInput{Name: name})
	if err != nil {
		t.Fatalf("seedClient %q: %v", name, err)
	}
	return p.ID, p.UUID
}

// seedDraftInvoice inserts a minimal draft invoice and returns its id.
func seedDraftInvoice(t *testing.T, conn *sql.DB, tenantID, clientID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	inv, err := gen.New(conn).CreateInvoice(context.Background(), gen.CreateInvoiceParams{
		Uuid: uuid.NewString(), TenantID: tenantID, Number: uuid.NewString(), ClientID: clientID,
		Status: "draft", IssueDate: "2026-01-01", DueDate: "2026-02-01", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedDraftInvoice: %v", err)
	}
	return inv.ID
}

// seedSentPastDue creates an invoice, flips it to 'sent', and back-dates its
// due_date into the past so the overdue sweep selects it. Returns the invoice.
func seedSentPastDue(t *testing.T, conn *sql.DB, svc *Service, tenantID, clientID int64) *Invoice {
	t.Helper()
	ctx := tctx(tenantID)
	inv, err := svc.Create(ctx, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-01-15",
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

// newInvoiceSvc creates a migrated DB, seeds a tenant+client, and returns
// the invoice Service, Hub, tenantID, clientID.
func newInvoiceSvc(t *testing.T) (*Service, *realtime.Hub, int64, int64) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme NDIS")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	hub := realtime.NewHub()
	return NewService(conn, hub, session.NewService(conn, hub, NewInvoices(conn))), hub, tenantID, clientID
}

// makeInvoice creates a single invoice for the tenant/client and returns it.
func makeInvoice(t *testing.T, svc *Service, tenantID, clientID int64) *Invoice {
	t.Helper()
	inv, err := svc.Create(tctx(tenantID), InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-02-01",
	}, []billing.LineItemInput{{Description: "A", Quantity: 1, UnitPrice: 10}})
	if err != nil {
		t.Fatalf("makeInvoice: %v", err)
	}
	if inv == nil {
		t.Fatal("makeInvoice: nil invoice")
	}
	return inv
}

// mkInvoiceRepo creates a single-line invoice for repository tests.
func mkInvoiceRepo(t *testing.T, repo *InvoicesRepo, tid, pid int64, due string) *Invoice {
	t.Helper()
	inv, err := repo.Create(context.Background(), tid, InvoiceInput{
		ClientID: pid, IssueDate: "2026-01-01", DueDate: due,
	}, []billing.LineItemInput{{Description: "X", Quantity: 1, UnitPrice: 100}})
	if err != nil {
		t.Fatalf("Create invoice: %v", err)
	}
	return inv
}

// seedInvoiceRepo creates a minimal one-line invoice and returns its id (used by payment tests).
func seedInvoiceRepo(t *testing.T, conn *sql.DB, tenantID, clientID int64, unitPrice float64) int64 {
	t.Helper()
	inv, err := NewInvoices(conn).Create(context.Background(), tenantID, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-01-31",
	}, []billing.LineItemInput{{Description: "Service", Quantity: 1, UnitPrice: unitPrice}})
	if err != nil {
		t.Fatalf("seedInvoiceRepo: %v", err)
	}
	return inv.ID
}

// seedInvoiceSvc creates a single-line invoice (unit price 25, qty 1) for service payment tests.
func seedInvoiceSvc(t *testing.T, invoices *InvoicesRepo, tenantID, clientID int64) *Invoice {
	t.Helper()
	inv, err := invoices.Create(tctx(tenantID), tenantID, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-06-01", DueDate: "2026-07-01",
	}, []billing.LineItemInput{{Description: "Work", Quantity: 1, UnitPrice: 25}})
	if err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	return inv
}
