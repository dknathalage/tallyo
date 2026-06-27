package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/ids"
)

func mustUserDB(t *testing.T) *sql.DB {
	t.Helper()
	conn := appdb.OpenTestDB(t)
	return conn
}

// seedTenant creates a tenant and returns its id (users FK → tenants).
func seedTenant(t *testing.T, conn *sql.DB, name string) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		ID: ids.New(), Name: name, Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedTenant %q: %v", name, err)
	}
	return tn.ID
}

func TestUserCreateGetListDelete(t *testing.T) {
	conn := mustUserDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewUsers(conn)
	ctx := context.Background()

	n, err := repo.Count(ctx, tid)
	if err != nil || n != 0 {
		t.Fatalf("Count=%d err=%v want 0", n, err)
	}

	u, err := repo.Create(ctx, tid, "owner@x.com", "uid-owner", "Owner", "owner", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Role != "owner" || u.Email != "owner@x.com" || u.ID == "" || u.Name != "Owner" || u.TenantID != tid {
		t.Fatalf("bad user %+v", u)
	}

	got, err := repo.GetByEmail(ctx, tid, "owner@x.com")
	if err != nil || got == nil || got.ID != u.ID {
		t.Fatalf("GetByEmail %+v err=%v", got, err)
	}

	byID, err := repo.GetByID(ctx, tid, u.ID)
	if err != nil || byID == nil || byID.Email != "owner@x.com" {
		t.Fatalf("GetByID %+v err=%v", byID, err)
	}

	byUID, err := repo.GetByFirebaseUID(ctx, tid, "uid-owner")
	if err != nil || byUID == nil || byUID.ID != u.ID {
		t.Fatalf("GetByFirebaseUID %+v err=%v", byUID, err)
	}

	list, err := repo.List(ctx, tid)
	if err != nil || len(list) != 1 {
		t.Fatalf("List len=%d err=%v", len(list), err)
	}

	if err := repo.TouchLastLogin(ctx, u.ID); err != nil {
		t.Fatalf("TouchLastLogin: %v", err)
	}

	if err := repo.Delete(ctx, tid, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	n, _ = repo.Count(ctx, tid)
	if n != 0 {
		t.Fatalf("after delete Count=%d want 0", n)
	}

	// mutations audited: create + delete => 2 user audit rows
	var rows int
	if err := conn.QueryRow("SELECT COUNT(*) FROM audit_log WHERE entity_type='user'").Scan(&rows); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if rows != 2 {
		t.Fatalf("user audit rows=%d want 2 (create+delete)", rows)
	}
}

func TestGetByEmailMissingReturnsNil(t *testing.T) {
	conn := mustUserDB(t)
	tid := seedTenant(t, conn, "T")
	got, err := NewUsers(conn).GetByEmail(context.Background(), tid, "no@x.com")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func TestCreateRejectsEmptyEmailOrUID(t *testing.T) {
	conn := mustUserDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewUsers(conn)
	ctx := context.Background()
	if _, err := repo.Create(ctx, tid, "", "uid", "N", "owner", false); err == nil {
		t.Fatal("empty email must error")
	}
	if _, err := repo.Create(ctx, tid, "a@x.com", "", "N", "owner", false); err == nil {
		t.Fatal("empty firebase uid must error")
	}
}

func TestUserTenantIsolation(t *testing.T) {
	conn := mustUserDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewUsers(conn)
	ctx := context.Background()

	u, err := repo.Create(ctx, a, "owner@x.com", "uid-a", "Owner", "owner", false)
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Tenant B cannot read tenant A's user by id, email or firebase uid.
	if got, _ := repo.GetByID(ctx, b, u.ID); got != nil {
		t.Fatalf("tenant B read tenant A's user by id: %+v", got)
	}
	if got, _ := repo.GetByEmail(ctx, b, "owner@x.com"); got != nil {
		t.Fatalf("tenant B read tenant A's user by email: %+v", got)
	}
	if got, _ := repo.GetByFirebaseUID(ctx, b, "uid-a"); got != nil {
		t.Fatalf("tenant B read tenant A's user by firebase uid: %+v", got)
	}
	if n, _ := repo.Count(ctx, b); n != 0 {
		t.Fatalf("tenant B Count = %d, want 0", n)
	}
}
