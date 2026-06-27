package client

import (
	"context"
	"database/sql"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/listquery"
	"github.com/dknathalage/tallyo/internal/payer"
	"net/url"
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

func TestClientCreateGet(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewClients(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, tid, ClientInput{
		Name: "Jane", Reference: "430000001",
		Email: "j@x.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p == nil || p.ID == "" || p.Name != "Jane" || p.Reference != "430000001" || p.Email != "j@x.com" {
		t.Fatalf("Create = %+v", p)
	}
	got, err := repo.Get(ctx, tid, p.ID)
	if err != nil || got == nil || got.Reference != "430000001" {
		t.Fatalf("Get = %+v err=%v", got, err)
	}
}

func TestClientWithPayerName(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	ctx := context.Background()
	pm, err := payer.NewPayers(conn).Create(ctx, tid, payer.PayerInput{Name: "PM Co"})
	if err != nil {
		t.Fatalf("Create PM: %v", err)
	}
	repo := NewClients(conn)
	p, err := repo.Create(ctx, tid, ClientInput{Name: "Jane", PayerUUID: &pm.ID})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.PayerName != "PM Co" {
		t.Fatalf("PayerName = %q, want PM Co", p.PayerName)
	}
}

func TestClientSearch(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewClients(conn)
	ctx := context.Background()

	if _, err := repo.Create(ctx, tid, ClientInput{Name: "Alice", Reference: "111"}); err != nil {
		t.Fatalf("Create Alice: %v", err)
	}
	if _, err := repo.Create(ctx, tid, ClientInput{Name: "Bob", Reference: "222"}); err != nil {
		t.Fatalf("Create Bob: %v", err)
	}
	// match on reference
	res, err := repo.List(ctx, tid, "222")
	if err != nil || len(res) != 1 || res[0].Name != "Bob" {
		t.Fatalf("search reference = %+v err=%v", res, err)
	}
}

func TestClientUpdateDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewClients(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, tid, ClientInput{Name: "Jane"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := repo.Update(ctx, tid, p.ID, ClientInput{Name: "Janet", Reference: "ref-1"})
	if err != nil || up == nil || up.Name != "Janet" || up.Reference != "ref-1" {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if err := repo.Delete(ctx, tid, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, p.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestClientTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewClients(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, a, ClientInput{Name: "Tenant A Person"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Tenant B cannot read tenant A's client.
	if got, _ := repo.Get(ctx, b, p.ID); got != nil {
		t.Fatalf("tenant B read tenant A's client: %+v", got)
	}
	if list, _ := repo.List(ctx, b, ""); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
	// Tenant B's update of A's row must not affect it (no rows match → nil).
	if got, _ := repo.Update(ctx, b, p.ID, ClientInput{Name: "Hijack"}); got != nil {
		t.Fatalf("tenant B updated tenant A's client: %+v", got)
	}
	stillA, _ := repo.Get(ctx, a, p.ID)
	if stillA == nil || stillA.Name != "Tenant A Person" {
		t.Fatalf("tenant A's client was mutated cross-tenant: %+v", stillA)
	}
}

func TestClientBulkDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewClients(conn)
	ctx := context.Background()

	a, _ := repo.Create(ctx, tid, ClientInput{Name: "Alice"})
	b, _ := repo.Create(ctx, tid, ClientInput{Name: "Bob"})
	c, _ := repo.Create(ctx, tid, ClientInput{Name: "Carol"})

	// Empty slice is a no-op.
	if err := repo.BulkDelete(ctx, tid, nil); err != nil {
		t.Fatalf("BulkDelete empty: %v", err)
	}
	if err := repo.BulkDelete(ctx, tid, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, _ := repo.List(ctx, tid, "")
	if len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("after bulk delete = %+v, want only Carol (id=%s)", list, c.ID)
	}
}

// TestClientQuery exercises the listquery-backed Query: text filter, sort
// direction, paging, and the total count (which ignores pagination).
func TestClientQuery(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewClients(conn)
	ctx := context.Background()

	for _, n := range []struct {
		name, ref string
	}{{"Amy", "P"}, {"Bob", "S"}, {"Cara", "P"}, {"Dan", "P"}} {
		if _, err := repo.Create(ctx, tid, ClientInput{Name: n.name, Reference: n.ref}); err != nil {
			t.Fatalf("Create %s: %v", n.name, err)
		}
	}

	// Filter reference=P (3 rows), sort name desc, limit 2 page 1.
	c := listquery.Build(mustVals(t, "f.reference=P&sort=name&dir=desc&limit=2&page=1"), ClientCols)
	rows, total, err := repo.Query(ctx, tid, c)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3 (reference P)", total)
	}
	if len(rows) != 2 {
		t.Fatalf("page len = %d, want 2", len(rows))
	}
	if rows[0].Name != "Dan" || rows[1].Name != "Cara" {
		t.Fatalf("desc page = [%q,%q], want [Dan,Cara]", rows[0].Name, rows[1].Name)
	}

	// Page 2 returns the remaining reference-P row (Amy).
	c2 := listquery.Build(mustVals(t, "f.reference=P&sort=name&dir=desc&limit=2&page=2"), ClientCols)
	rows2, _, err := repo.Query(ctx, tid, c2)
	if err != nil || len(rows2) != 1 || rows2[0].Name != "Amy" {
		t.Fatalf("page 2 = %+v err=%v, want [Amy]", rows2, err)
	}

	// Text filter on name.
	c3 := listquery.Build(mustVals(t, "f.name=ar"), ClientCols)
	rows3, total3, err := repo.Query(ctx, tid, c3)
	if err != nil || total3 != 1 || len(rows3) != 1 || rows3[0].Name != "Cara" {
		t.Fatalf("name contains 'ar' = %+v total=%d err=%v, want [Cara]", rows3, total3, err)
	}
}

func mustVals(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	return v
}

// TestClientListPlain exercises the no-search List path (toClientList),
// asserting ordering by name and that fields round-trip.
func TestClientListPlain(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewClients(conn)
	ctx := context.Background()

	if _, err := repo.Create(ctx, tid, ClientInput{Name: "Zoe", Reference: "999"}); err != nil {
		t.Fatalf("Create Zoe: %v", err)
	}
	if _, err := repo.Create(ctx, tid, ClientInput{Name: "Amy", Email: "amy@x.com"}); err != nil {
		t.Fatalf("Create Amy: %v", err)
	}

	list, err := repo.List(ctx, tid, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	// Ordered by name: Amy before Zoe.
	if list[0].Name != "Amy" || list[1].Name != "Zoe" {
		t.Fatalf("order = [%q, %q], want [Amy, Zoe]", list[0].Name, list[1].Name)
	}
	if list[0].Email != "amy@x.com" {
		t.Fatalf("Amy email = %q, want amy@x.com", list[0].Email)
	}
}
