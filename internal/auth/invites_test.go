package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

// mustInviteDB returns a migrated DB, a tenant id, and the id of a pre-created
// owner user, which is required to satisfy the invites.created_by foreign key.
func mustInviteDB(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	tid := seedTenant(t, conn, "T")
	hash, err := HashPassword("pw123456")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	owner, err := NewUsers(conn).Create(context.Background(), tid, "owner@x.com", hash, "Owner", "owner", false)
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	return conn, tid, owner.ID
}

func TestAcceptInviteAtomic(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	ctx := context.Background()
	invRepo := NewInvites(conn)
	usersRepo := NewUsers(conn)
	inv, err := invRepo.Create(ctx, tid, "new@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create invite: %v", err)
	}

	hash, _ := HashPassword("password1")
	u, err := invRepo.Accept(ctx, inv.Token, "New User", hash)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if u == nil || u.Email != "new@x.com" || u.Role != "member" || u.TenantID != tid || u.Name != "New User" {
		t.Fatalf("bad user %+v", u)
	}

	// invite now accepted → second accept fails
	if _, err := invRepo.Accept(ctx, inv.Token, "X", hash); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("second accept: want ErrInviteInvalid, got %v", err)
	}
	// user actually created in the invite's tenant
	got, _ := usersRepo.GetByEmail(ctx, tid, "new@x.com")
	if got == nil {
		t.Fatal("user not created")
	}
}

func TestAcceptInviteDuplicateEmail(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	ctx := context.Background()
	invRepo := NewInvites(conn)
	usersRepo := NewUsers(conn)
	// pre-existing user with the invited email in the same tenant
	if _, err := usersRepo.Create(ctx, tid, "dup@x.com", "h", "Dup", "member", false); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	inv, _ := invRepo.Create(ctx, tid, "dup@x.com", "member", owner, time.Hour)

	hash, _ := HashPassword("password1")
	_, err := invRepo.Accept(ctx, inv.Token, "Dup2", hash)
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
}

func TestInviteCreateAndGet(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, "staff@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.Token == "" {
		t.Fatal("token must be non-empty")
	}
	if inv.Email != "staff@x.com" || inv.Role != "member" || inv.ID == "" || inv.TenantID != tid {
		t.Fatalf("bad invite %+v", inv)
	}
	exp, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Fatalf("expires_at %s not in future", inv.ExpiresAt)
	}

	got, err := repo.GetByToken(ctx, inv.Token)
	if err != nil || got == nil || got.ID != inv.ID {
		t.Fatalf("GetByToken %+v err=%v", got, err)
	}

	miss, err := repo.GetByToken(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetByToken unknown err=%v", err)
	}
	if miss != nil {
		t.Fatalf("unknown token should be nil, got %+v", miss)
	}
}

func TestInviteValidateFresh(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, "staff@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.Validate(ctx, inv.Token)
	if err != nil {
		t.Fatalf("Validate fresh: %v", err)
	}
	if got == nil || got.ID != inv.ID {
		t.Fatalf("Validate returned %+v", got)
	}
}

func TestInviteValidateUnknownToken(t *testing.T) {
	conn, _, _ := mustInviteDB(t)
	defer conn.Close()
	_, err := NewInvites(conn).Validate(context.Background(), "nope")
	if !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("want ErrInviteInvalid, got %v", err)
	}
}

func TestInviteValidateExpired(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, "staff@x.com", "member", owner, -1*time.Minute)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = repo.Validate(ctx, inv.Token)
	if !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("want ErrInviteInvalid for expired, got %v", err)
	}
}

func TestInviteMarkAcceptedThenValidateFails(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, "staff@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.MarkAccepted(ctx, inv.Token); err != nil {
		t.Fatalf("MarkAccepted: %v", err)
	}
	got, err := repo.GetByToken(ctx, inv.Token)
	if err != nil || got == nil {
		t.Fatalf("GetByToken after accepted: %+v err=%v", got, err)
	}
	if !got.Accepted {
		t.Fatal("invite should be marked Accepted")
	}
	_, err = repo.Validate(ctx, inv.Token)
	if !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("want ErrInviteInvalid for accepted, got %v", err)
	}
}

func TestInviteTokensDiffer(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()

	a, err := repo.Create(ctx, tid, "a@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create a: %v", err)
	}
	b, err := repo.Create(ctx, tid, "b@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create b: %v", err)
	}
	if a.Token == b.Token {
		t.Fatalf("tokens must differ: %s", a.Token)
	}
}

func TestInviteCreateRejectsEmptyEmail(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	if _, err := NewInvites(conn).Create(context.Background(), tid, "", "member", owner, time.Hour); err == nil {
		t.Fatal("empty email must error")
	}
}

func TestInviteExposesUUIDAsID(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	inv, err := NewInvites(conn).Create(context.Background(), tid, "u@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// The DTO carries the uuid (the public id), distinct from the int PK.
	if inv.ID == "" {
		t.Fatal("invite UUID must be non-empty")
	}
	b, err := json.Marshal(inv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["id"] != inv.ID {
		t.Fatalf("json id=%v want uuid %q", m["id"], inv.ID)
	}
	// The int PK and tenant id must not leak.
	if _, ok := m["tenantId"]; ok {
		t.Fatalf("tenantId must not be in JSON: %v", m)
	}
	if s, ok := m["id"].(string); !ok || s == "" {
		t.Fatalf("id must be a non-empty uuid string, got %v", m["id"])
	}
}

func TestDeleteInviteByUUID(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, "rm@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.DeleteByUUID(ctx, tid, inv.ID); err != nil {
		t.Fatalf("DeleteByUUID: %v", err)
	}
	// gone: the token no longer resolves.
	got, err := repo.GetByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("GetByToken after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("invite should be deleted, got %+v", got)
	}
}

func TestDeleteInviteByUUIDUnknownIsNoOp(t *testing.T) {
	conn, tid, _ := mustInviteDB(t)
	defer conn.Close()
	// A well-formed but unknown uuid matches no rows → no error (idempotent).
	if err := NewInvites(conn).DeleteByUUID(context.Background(), tid, "3f1b8e2a-6c4d-4f7a-9b0c-1d2e3f4a5b6c"); err != nil {
		t.Fatalf("unknown uuid delete should be no-op, got %v", err)
	}
}

func TestDeleteInviteByUUIDTenantScoped(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()
	inv, err := repo.Create(ctx, tid, "scope@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	otherTenant := seedTenant(t, conn, "Other")
	// Deleting under the wrong tenant must not remove the invite.
	if err := repo.DeleteByUUID(ctx, otherTenant, inv.ID); err != nil {
		t.Fatalf("DeleteByUUID other tenant: %v", err)
	}
	got, err := repo.GetByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil {
		t.Fatal("invite must survive a cross-tenant delete")
	}
}

func TestInviteAuditRows(t *testing.T) {
	conn, tid, owner := mustInviteDB(t)
	defer conn.Close()
	repo := NewInvites(conn)
	ctx := context.Background()

	inv, err := repo.Create(ctx, tid, "staff@x.com", "member", owner, time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.MarkAccepted(ctx, inv.Token); err != nil {
		t.Fatalf("MarkAccepted: %v", err)
	}

	var created, accepted int
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='invite' AND action='create'").Scan(&created); err != nil {
		t.Fatalf("audit create count: %v", err)
	}
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='invite' AND action='accepted'").Scan(&accepted); err != nil {
		t.Fatalf("audit accepted count: %v", err)
	}
	if created != 1 {
		t.Fatalf("invite create audit rows=%d want 1", created)
	}
	if accepted != 1 {
		t.Fatalf("invite accepted audit rows=%d want 1", accepted)
	}
}
