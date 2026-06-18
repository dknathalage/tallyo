package participant

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/planmanager"
	"github.com/google/uuid"
)

// newTestDB opens a fresh migrated in-temp SQLite DB.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "participant.db"))
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

func TestParticipantCreateGet(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, tid, ParticipantInput{
		Name: "Jane", NDISNumber: "430000001", PlanStart: "2026-01-01", PlanEnd: "2026-12-31", MgmtType: "self",
		Email: "j@x.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p == nil || p.ID == 0 || p.Name != "Jane" || p.NDISNumber != "430000001" || p.MgmtType != "self" {
		t.Fatalf("Create = %+v", p)
	}
	got, err := repo.Get(ctx, tid, p.ID)
	if err != nil || got == nil || got.PlanEnd != "2026-12-31" {
		t.Fatalf("Get = %+v err=%v", got, err)
	}
}

func TestParticipantDefaultMgmtType(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	p, err := NewParticipants(conn).Create(context.Background(), tid, ParticipantInput{Name: "Jane"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.MgmtType != "plan" {
		t.Fatalf("default mgmtType = %q, want plan", p.MgmtType)
	}
}

func TestParticipantWithPlanManagerName(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	ctx := context.Background()
	pm, err := planmanager.NewPlanManagers(conn).Create(ctx, tid, planmanager.PlanManagerInput{Name: "PM Co"})
	if err != nil {
		t.Fatalf("Create PM: %v", err)
	}
	repo := NewParticipants(conn)
	p, err := repo.Create(ctx, tid, ParticipantInput{Name: "Jane", MgmtType: "plan", PlanManagerID: &pm.ID})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.PlanManagerName != "PM Co" {
		t.Fatalf("PlanManagerName = %q, want PM Co", p.PlanManagerName)
	}
}

func TestParticipantSearch(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	if _, err := repo.Create(ctx, tid, ParticipantInput{Name: "Alice", NDISNumber: "111"}); err != nil {
		t.Fatalf("Create Alice: %v", err)
	}
	if _, err := repo.Create(ctx, tid, ParticipantInput{Name: "Bob", NDISNumber: "222"}); err != nil {
		t.Fatalf("Create Bob: %v", err)
	}
	// match on ndis number
	res, err := repo.List(ctx, tid, "222")
	if err != nil || len(res) != 1 || res[0].Name != "Bob" {
		t.Fatalf("search ndis = %+v err=%v", res, err)
	}
}

func TestParticipantUpdateDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, tid, ParticipantInput{Name: "Jane"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := repo.Update(ctx, tid, p.ID, ParticipantInput{Name: "Janet", MgmtType: "self"})
	if err != nil || up == nil || up.Name != "Janet" || up.MgmtType != "self" {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if err := repo.Delete(ctx, tid, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, p.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestParticipantTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewParticipants(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, a, ParticipantInput{Name: "Tenant A Person"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Tenant B cannot read tenant A's participant.
	if got, _ := repo.Get(ctx, b, p.ID); got != nil {
		t.Fatalf("tenant B read tenant A's participant: %+v", got)
	}
	if list, _ := repo.List(ctx, b, ""); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
	// Tenant B's update of A's row must not affect it (no rows match → nil).
	if got, _ := repo.Update(ctx, b, p.ID, ParticipantInput{Name: "Hijack"}); got != nil {
		t.Fatalf("tenant B updated tenant A's participant: %+v", got)
	}
	stillA, _ := repo.Get(ctx, a, p.ID)
	if stillA == nil || stillA.Name != "Tenant A Person" {
		t.Fatalf("tenant A's participant was mutated cross-tenant: %+v", stillA)
	}
}

func TestParticipantBulkDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	a, _ := repo.Create(ctx, tid, ParticipantInput{Name: "Alice"})
	b, _ := repo.Create(ctx, tid, ParticipantInput{Name: "Bob"})
	c, _ := repo.Create(ctx, tid, ParticipantInput{Name: "Carol"})

	// Empty slice is a no-op.
	if err := repo.BulkDelete(ctx, tid, nil); err != nil {
		t.Fatalf("BulkDelete empty: %v", err)
	}
	if err := repo.BulkDelete(ctx, tid, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	list, _ := repo.List(ctx, tid, "")
	if len(list) != 1 || list[0].ID != c.ID {
		t.Fatalf("after bulk delete = %+v, want only Carol (id=%d)", list, c.ID)
	}
}

// TestParticipantListPlain exercises the no-search List path (toParticipantList),
// asserting ordering by name and that fields round-trip.
func TestParticipantListPlain(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	if _, err := repo.Create(ctx, tid, ParticipantInput{Name: "Zoe", NDISNumber: "999"}); err != nil {
		t.Fatalf("Create Zoe: %v", err)
	}
	if _, err := repo.Create(ctx, tid, ParticipantInput{Name: "Amy", Email: "amy@x.com"}); err != nil {
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
