package session

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
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/reqctx"
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
	p, err := client.NewClients(conn).Create(context.Background(), tenantID, client.ClientInput{Name: name})
	if err != nil {
		t.Fatalf("seedClient %q: %v", name, err)
	}
	return p.ID
}

// seedUser inserts a member user for the tenant and returns its id.
func seedUser(t *testing.T, conn *sql.DB, tenantID string) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	u, err := gen.New(conn).CreateUser(context.Background(), gen.CreateUserParams{
		ID: ids.New(), TenantID: tenantID, Email: ids.New() + "@x.com",
		FirebaseUid: ids.New(), Name: "U", Role: "member", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	return u.ID
}

// seedInvoice creates a minimal one-line invoice and returns its id.
func seedInvoice(t *testing.T, conn *sql.DB, tenantID, clientID string, unitPrice float64) string {
	t.Helper()
	inv, err := invoice.NewInvoices(conn).Create(context.Background(), tenantID, invoice.InvoiceInput{
		ClientID: clientID, IssueDate: "2026-01-01", DueDate: "2026-01-31",
	}, []billing.LineItemInput{{Description: "Service", Quantity: 1, UnitPrice: unitPrice}})
	if err != nil {
		t.Fatalf("seedInvoice: %v", err)
	}
	return inv.ID
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

// containsID reports whether ids contains target.
func containsID(ids []string, target string) bool {
	for i := range ids { // bounded by len(ids)
		if ids[i] == target {
			return true
		}
	}
	return false
}
