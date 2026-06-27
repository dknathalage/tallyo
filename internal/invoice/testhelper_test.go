package invoice

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/billing"
	"github.com/dknathalage/tallyo/internal/client"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/session"
)

// newTestDB opens the shared migrated Postgres test DB.
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

// tctx returns a context carrying the given tenant id.
func tctx(tenantID string) context.Context {
	return reqctx.WithTenant(context.Background(), tenantID)
}

// seedClient inserts a minimal client for a tenant and returns its id.
func seedClient(t *testing.T, conn *sql.DB, tenantID string, name string) string {
	t.Helper()
	id, _ := seedClientUUID(t, conn, tenantID, name)
	return id
}

// seedClientUUID seeds a client and returns its (id, uuid). With uuid ids the
// row id IS the public uuid, so both returns are the same value.
func seedClientUUID(t *testing.T, conn *sql.DB, tenantID string, name string) (string, string) {
	t.Helper()
	p, err := client.NewClients(conn).Create(context.Background(), tenantID, client.ClientInput{Name: name})
	if err != nil {
		t.Fatalf("seedClient %q: %v", name, err)
	}
	return p.ID, p.ID
}

// seedDraftInvoice inserts a minimal draft invoice and returns its id.
func seedDraftInvoice(t *testing.T, conn *sql.DB, tenantID, clientID string) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	inv, err := gen.New(conn).CreateInvoice(context.Background(), gen.CreateInvoiceParams{
		ID: ids.New(), TenantID: tenantID, Number: ids.New(), ClientID: clientID,
		Status: "draft", IssueDate: "2026-01-01", DueDate: "2026-02-01", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedDraftInvoice: %v", err)
	}
	return inv.ID
}

// newInvoiceSvc creates a migrated DB, seeds a tenant+client, and returns
// the invoice Service, tenantID, clientID.
func newInvoiceSvc(t *testing.T) (*Service, string, string) {
	t.Helper()
	conn := newTestDB(t)
	tenantID := seedTenant(t, conn, "Acme")
	clientID := seedClient(t, conn, tenantID, "Jane Client")
	return NewService(conn, session.NewService(conn, NewInvoices(conn))), tenantID, clientID
}

// makeInvoice creates a single invoice for the tenant/client and returns it.
func makeInvoice(t *testing.T, svc *Service, tenantID, clientID string) *Invoice {
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
func mkInvoiceRepo(t *testing.T, repo *InvoicesRepo, tid, pid string, due string) *Invoice {
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
func seedInvoiceRepo(t *testing.T, conn *sql.DB, tenantID, clientID string, unitPrice float64) string {
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
func seedInvoiceSvc(t *testing.T, invoices *InvoicesRepo, tenantID, clientID string) *Invoice {
	t.Helper()
	inv, err := invoices.Create(tctx(tenantID), tenantID, InvoiceInput{
		ClientID: clientID, IssueDate: "2026-06-01", DueDate: "2026-07-01",
	}, []billing.LineItemInput{{Description: "Work", Quantity: 1, UnitPrice: 25}})
	if err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	return inv
}
