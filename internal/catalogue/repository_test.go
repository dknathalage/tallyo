package catalogue

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "catalogue.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}

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

// referenceItem inserts an invoice line referencing the given catalogue version
// row id, simulating a billed document so copy-on-write forks on the next edit.
func referenceItem(t *testing.T, conn *sql.DB, tenantID, catalogueItemID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	cliID := ids.New()
	if _, err := conn.Exec(
		"INSERT INTO clients (id, tenant_id, name, created_at, updated_at) VALUES (?,?,?,?,?)",
		cliID, tenantID, "Client", now, now,
	); err != nil {
		t.Fatalf("insert client: %v", err)
	}
	invID := ids.New()
	if _, err := conn.Exec(
		"INSERT INTO invoices (id, tenant_id, number, client_id, issue_date, due_date, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)",
		invID, tenantID, "INV-"+invID[:8], cliID, now[:10], now[:10], now, now,
	); err != nil {
		t.Fatalf("insert invoice: %v", err)
	}
	if _, err := conn.Exec(
		"INSERT INTO line_items (id, tenant_id, invoice_id, catalogue_item_id, description, quantity, unit_price) VALUES (?,?,?,?,?,?,?)",
		ids.New(), tenantID, invID, catalogueItemID, "billed work", 1, 10,
	); err != nil {
		t.Fatalf("insert line_item: %v", err)
	}
}

func TestCatalogueCRUD(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	ci, err := repo.Create(ctx, tid, CatalogueItemInput{Code: "TR", Name: "Travel", UnitPrice: 1.5, Unit: "km", Category: "logistics", Taxable: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ci.ID == "" || ci.LogicalID != ci.ID || ci.Version != 1 || !ci.IsCurrent || ci.UnitPrice != 1.5 {
		t.Fatalf("Create = %+v", ci)
	}
	got, err := repo.Get(ctx, tid, ci.ID)
	if err != nil || got == nil || got.Name != "Travel" || got.Category != "logistics" {
		t.Fatalf("Get = %+v err=%v", got, err)
	}
	if list, _ := repo.List(ctx, tid); len(list) != 1 {
		t.Fatalf("List len = %d, want 1", len(list))
	}
	if err := repo.Delete(ctx, tid, ci.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if list, _ := repo.List(ctx, tid); len(list) != 0 {
		t.Fatalf("List after delete len = %d, want 0 (tombstoned)", len(list))
	}
}

// Updating an UNREFERENCED item mutates the current row in place (same id, no new version).
func TestCatalogueUpdateInPlace(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	ci, _ := repo.Create(ctx, tid, CatalogueItemInput{Name: "Widget", UnitPrice: 5})
	up, err := repo.Update(ctx, tid, ci.ID, CatalogueItemInput{Name: "Widget", UnitPrice: 7})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if up.ID != ci.ID {
		t.Fatalf("in-place update changed id: %s -> %s", ci.ID, up.ID)
	}
	if up.Version != 1 || up.UnitPrice != 7 {
		t.Fatalf("Update = %+v, want version 1 price 7", up)
	}
	if list, _ := repo.List(ctx, tid); len(list) != 1 {
		t.Fatalf("List len = %d, want 1 (no fork)", len(list))
	}
}

// Updating a REFERENCED item forks a new version; the old row stays frozen at its
// old price (an existing invoice keeps its pinned version).
func TestCatalogueUpdateForksWhenReferenced(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	ci, _ := repo.Create(ctx, tid, CatalogueItemInput{Name: "Service", UnitPrice: 100})
	referenceItem(t, conn, tid, ci.ID)

	up, err := repo.Update(ctx, tid, ci.ID, CatalogueItemInput{Name: "Service", UnitPrice: 120})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if up.ID == ci.ID {
		t.Fatalf("referenced update should fork a new row, got same id %s", up.ID)
	}
	if up.LogicalID != ci.LogicalID || up.Version != 2 || up.UnitPrice != 120 || !up.IsCurrent {
		t.Fatalf("forked = %+v, want same logical, version 2, price 120, current", up)
	}
	// Old row frozen at 100, no longer current.
	old, err := repo.GetByID(ctx, tid, ci.ID)
	if err != nil {
		t.Fatalf("GetByID old: %v", err)
	}
	if old.UnitPrice != 100 || old.IsCurrent {
		t.Fatalf("old row = %+v, want price 100 not current", old)
	}
	// Exactly one current version in the catalogue list.
	if list, _ := repo.List(ctx, tid); len(list) != 1 || list[0].ID != up.ID {
		t.Fatalf("List = %+v, want only the forked current row", list)
	}
}

func TestCatalogueTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn)
	b := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	ci, _ := repo.Create(ctx, a, CatalogueItemInput{Name: "A item", UnitPrice: 1})
	if got, _ := repo.Get(ctx, b, ci.ID); got != nil {
		t.Fatalf("tenant B read tenant A's item: %+v", got)
	}
	if list, _ := repo.List(ctx, b); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
}

func TestCatalogueSearch(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn)
	repo := NewRepo(conn)
	ctx := context.Background()

	_, _ = repo.Create(ctx, tid, CatalogueItemInput{Code: "AB1", Name: "Alpha", Category: "tools", UnitPrice: 1})
	_, _ = repo.Create(ctx, tid, CatalogueItemInput{Code: "XY2", Name: "Beta", Category: "parts", UnitPrice: 2})

	got, err := repo.Search(ctx, tid, "tools")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Alpha" {
		t.Fatalf("Search(tools) = %+v, want only Alpha", got)
	}
}
