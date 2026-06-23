package shift

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
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/google/uuid"
)

// newTestDB opens a fresh migrated in-temp SQLite DB.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "shift.db"))
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

// seedClient inserts a minimal client for a tenant and returns its id.
func seedClient(t *testing.T, conn *sql.DB, tenantID int64, name string) int64 {
	t.Helper()
	p, err := client.NewClients(conn).Create(context.Background(), tenantID, client.ClientInput{Name: name})
	if err != nil {
		t.Fatalf("seedClient %q: %v", name, err)
	}
	return p.ID
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

// seedInvoice creates a minimal one-line invoice and returns its id.
func seedInvoice(t *testing.T, conn *sql.DB, tenantID, clientID int64, unitPrice float64) int64 {
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

// containsID reports whether ids contains target.
func containsID(ids []int64, target int64) bool {
	for i := range ids { // bounded by len(ids)
		if ids[i] == target {
			return true
		}
	}
	return false
}
