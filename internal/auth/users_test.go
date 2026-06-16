package auth

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/google/uuid"
)

func mustUserDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "u.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}

// seedTenant creates a tenant and returns its id (users FK → tenants).
func seedTenant(t *testing.T, conn *sql.DB, name string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	tn, err := gen.New(conn).CreateTenant(context.Background(), gen.CreateTenantParams{
		Uuid: uuid.NewString(), Name: name, Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("seedTenant %q: %v", name, err)
	}
	return tn.ID
}

func TestUserCreateGetListDelete(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	tid := seedTenant(t, conn, "T")
	repo := NewUsers(conn)
	ctx := context.Background()

	n, err := repo.Count(ctx, tid)
	if err != nil || n != 0 {
		t.Fatalf("Count=%d err=%v want 0", n, err)
	}

	hash, _ := HashPassword("pw123456")
	u, err := repo.Create(ctx, tid, "owner@x.com", hash, "Owner", "owner", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Role != "owner" || u.Email != "owner@x.com" || u.ID == 0 || u.Name != "Owner" || u.TenantID != tid {
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

	list, err := repo.List(ctx, tid)
	if err != nil || len(list) != 1 {
		t.Fatalf("List len=%d err=%v", len(list), err)
	}

	// global credentials lookup for login (pre-tenant)
	creds, found, err := repo.GetCredentialsGlobal(ctx, "owner@x.com")
	if err != nil || !found || creds.ID != u.ID || creds.TenantID != tid || creds.Hash != hash {
		t.Fatalf("GetCredentialsGlobal %+v found=%v err=%v", creds, found, err)
	}
	_, found, _ = repo.GetCredentialsGlobal(ctx, "nobody@x.com")
	if found {
		t.Fatal("GetCredentialsGlobal should not find unknown email")
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
	defer conn.Close()
	tid := seedTenant(t, conn, "T")
	got, err := NewUsers(conn).GetByEmail(context.Background(), tid, "no@x.com")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func TestCreateRejectsEmptyEmailOrHash(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	tid := seedTenant(t, conn, "T")
	repo := NewUsers(conn)
	ctx := context.Background()
	if _, err := repo.Create(ctx, tid, "", "h", "N", "owner", false); err == nil {
		t.Fatal("empty email must error")
	}
	if _, err := repo.Create(ctx, tid, "a@x.com", "", "N", "owner", false); err == nil {
		t.Fatal("empty hash must error")
	}
}

func TestUserTenantIsolation(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewUsers(conn)
	ctx := context.Background()

	hash, _ := HashPassword("pw123456")
	u, err := repo.Create(ctx, a, "owner@x.com", hash, "Owner", "owner", false)
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Tenant B cannot read tenant A's user by id or email.
	if got, _ := repo.GetByID(ctx, b, u.ID); got != nil {
		t.Fatalf("tenant B read tenant A's user by id: %+v", got)
	}
	if got, _ := repo.GetByEmail(ctx, b, "owner@x.com"); got != nil {
		t.Fatalf("tenant B read tenant A's user by email: %+v", got)
	}
	if n, _ := repo.Count(ctx, b); n != 0 {
		t.Fatalf("tenant B Count = %d, want 0", n)
	}
	// But the global lookup (login) still finds it.
	if g, _ := repo.GetByEmailGlobal(ctx, "owner@x.com"); g == nil || g.TenantID != a {
		t.Fatalf("GetByEmailGlobal = %+v, want tenant A's user", g)
	}
}
