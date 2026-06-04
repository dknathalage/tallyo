package auth

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
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

func TestUserCreateGetListDelete(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	repo := NewUsers(conn)
	ctx := context.Background()

	n, err := repo.Count(ctx)
	if err != nil || n != 0 {
		t.Fatalf("Count=%d err=%v want 0", n, err)
	}

	hash, _ := HashPassword("pw123456")
	u, err := repo.Create(ctx, "owner@x.com", hash, "owner")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Role != "owner" || u.Email != "owner@x.com" || u.ID == 0 {
		t.Fatalf("bad user %+v", u)
	}

	got, err := repo.GetByEmail(ctx, "owner@x.com")
	if err != nil || got == nil || got.ID != u.ID {
		t.Fatalf("GetByEmail %+v err=%v", got, err)
	}

	byID, err := repo.GetByID(ctx, u.ID)
	if err != nil || byID == nil || byID.Email != "owner@x.com" {
		t.Fatalf("GetByID %+v err=%v", byID, err)
	}

	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("List len=%d err=%v", len(list), err)
	}

	// credentials lookup for login
	id, h, found, err := repo.GetCredentials(ctx, "owner@x.com")
	if err != nil || !found || id != u.ID || h != hash {
		t.Fatalf("GetCredentials id=%d found=%v err=%v", id, found, err)
	}
	_, _, found, _ = repo.GetCredentials(ctx, "nobody@x.com")
	if found {
		t.Fatal("GetCredentials should not find unknown email")
	}

	if err := repo.TouchLastLogin(ctx, u.ID); err != nil {
		t.Fatalf("TouchLastLogin: %v", err)
	}

	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	n, _ = repo.Count(ctx)
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
	got, err := NewUsers(conn).GetByEmail(context.Background(), "no@x.com")
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
	repo := NewUsers(conn)
	ctx := context.Background()
	if _, err := repo.Create(ctx, "", "h", "owner"); err == nil {
		t.Fatal("empty email must error")
	}
	if _, err := repo.Create(ctx, "a@x.com", "", "owner"); err == nil {
		t.Fatal("empty hash must error")
	}
}
