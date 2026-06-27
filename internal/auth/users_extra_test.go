package auth

import (
	"context"
	"testing"
)

func TestTenantsForFirebaseUID(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	a := seedTenant(t, conn, "Alpha")
	b := seedTenant(t, conn, "Beta")
	repo := NewUsers(conn)
	ctx := context.Background()

	// Same Firebase identity is a member of two tenants (flat user pool).
	if _, err := repo.Create(ctx, a, "shared@x.com", "uid-shared", "A", "owner", false); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := repo.Create(ctx, b, "shared@x.com", "uid-shared", "B", "owner", false); err != nil {
		t.Fatalf("create B: %v", err)
	}

	rows, err := repo.TenantsForFirebaseUID(ctx, "uid-shared")
	if err != nil {
		t.Fatalf("TenantsForFirebaseUID: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	seen := map[string]bool{}
	for i := range rows {
		if rows[i].TenantName == "" || rows[i].TenantUUID == "" {
			t.Fatalf("row %d missing name/uuid: %+v", i, rows[i])
		}
		seen[rows[i].TenantID] = true
	}
	if !seen[a] || !seen[b] {
		t.Fatalf("expected both tenants, seen=%v", seen)
	}
}

func TestTenantsForFirebaseUIDUnknownReturnsEmpty(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	rows, err := NewUsers(conn).TenantsForFirebaseUID(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("TenantsForFirebaseUID: %v", err)
	}
	if rows == nil {
		t.Fatal("rows must be non-nil slice")
	}
	if len(rows) != 0 {
		t.Fatalf("rows=%d want 0", len(rows))
	}
}

func TestGetByFirebaseUIDMissingReturnsNil(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	tid := seedTenant(t, conn, "T")
	got, err := NewUsers(conn).GetByFirebaseUID(context.Background(), tid, "nobody")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func TestUserCreateRejectsZeroTenant(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	if _, err := NewUsers(conn).Create(context.Background(), "", "a@x.com", "uid", "N", "owner", false); err == nil {
		t.Fatal("zero tenant id must error")
	}
}

func TestNewUsersNilDBPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewUsers(nil) must panic")
		}
	}()
	NewUsers(nil)
}
