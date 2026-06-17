package auth

import (
	"context"
	"testing"
)

func TestTenantsForEmail(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	a := seedTenant(t, conn, "Alpha")
	b := seedTenant(t, conn, "Beta")
	repo := NewUsers(conn)
	ctx := context.Background()

	hash, _ := HashPassword("pw123456")
	if _, err := repo.Create(ctx, a, "shared@x.com", hash, "A", "owner", false); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := repo.Create(ctx, b, "shared@x.com", hash, "B", "owner", false); err != nil {
		t.Fatalf("create B: %v", err)
	}

	rows, err := repo.TenantsForEmail(ctx, "shared@x.com")
	if err != nil {
		t.Fatalf("TenantsForEmail: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	// bounded loop: at most 2 rows
	seen := map[int64]bool{}
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

func TestTenantsForEmailUnknownReturnsEmpty(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	rows, err := NewUsers(conn).TenantsForEmail(context.Background(), "nobody@x.com")
	if err != nil {
		t.Fatalf("TenantsForEmail: %v", err)
	}
	if rows == nil {
		t.Fatal("rows must be non-nil slice")
	}
	if len(rows) != 0 {
		t.Fatalf("rows=%d want 0", len(rows))
	}
}

func TestGetCredentialsForTenant(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	a := seedTenant(t, conn, "Alpha")
	b := seedTenant(t, conn, "Beta")
	repo := NewUsers(conn)
	ctx := context.Background()

	hash, _ := HashPassword("pw123456")
	u, err := repo.Create(ctx, a, "user@x.com", hash, "U", "owner", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	creds, found, err := repo.GetCredentialsForTenant(ctx, a, "user@x.com")
	if err != nil {
		t.Fatalf("GetCredentialsForTenant: %v", err)
	}
	if !found {
		t.Fatal("found=false for existing (tenant,email)")
	}
	if creds.ID != u.ID || creds.TenantID != a || creds.Hash != hash {
		t.Fatalf("bad creds %+v", creds)
	}

	// the same email in a different tenant must NOT match.
	_, found, err = repo.GetCredentialsForTenant(ctx, b, "user@x.com")
	if err != nil {
		t.Fatalf("GetCredentialsForTenant other tenant: %v", err)
	}
	if found {
		t.Fatal("found=true for (wrong tenant, email)")
	}
}

func TestGetCredentialsForTenantRejectsZeroTenant(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	if _, _, err := NewUsers(conn).GetCredentialsForTenant(context.Background(), 0, "a@x.com"); err == nil {
		t.Fatal("zero tenant id must error")
	}
}

func TestGetByEmailGlobalMissingReturnsNil(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	got, err := NewUsers(conn).GetByEmailGlobal(context.Background(), "nobody@x.com")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != nil {
		t.Fatalf("want nil, got %+v", got)
	}
}

func TestCountByEmailGlobal(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	a := seedTenant(t, conn, "Alpha")
	b := seedTenant(t, conn, "Beta")
	repo := NewUsers(conn)
	ctx := context.Background()

	hash, _ := HashPassword("pw123456")
	if _, err := repo.Create(ctx, a, "dup@x.com", hash, "A", "owner", false); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := repo.Create(ctx, b, "dup@x.com", hash, "B", "owner", false); err != nil {
		t.Fatalf("create B: %v", err)
	}

	n, err := repo.CountByEmailGlobal(ctx, "dup@x.com")
	if err != nil {
		t.Fatalf("CountByEmailGlobal: %v", err)
	}
	if n != 2 {
		t.Fatalf("count=%d want 2", n)
	}

	zero, err := repo.CountByEmailGlobal(ctx, "nobody@x.com")
	if err != nil {
		t.Fatalf("CountByEmailGlobal unknown: %v", err)
	}
	if zero != 0 {
		t.Fatalf("count=%d want 0", zero)
	}
}

func TestGetCredentialsGlobalAmbiguous(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	a := seedTenant(t, conn, "Alpha")
	b := seedTenant(t, conn, "Beta")
	repo := NewUsers(conn)
	ctx := context.Background()

	hash, _ := HashPassword("pw123456")
	if _, err := repo.Create(ctx, a, "dup@x.com", hash, "A", "owner", false); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := repo.Create(ctx, b, "dup@x.com", hash, "B", "owner", false); err != nil {
		t.Fatalf("create B: %v", err)
	}

	_, found, err := repo.GetCredentialsGlobal(ctx, "dup@x.com")
	if err != ErrAmbiguousEmail {
		t.Fatalf("err=%v want ErrAmbiguousEmail", err)
	}
	if found {
		t.Fatal("found must be false on ambiguous email")
	}
}

func TestUserCreateRejectsZeroTenant(t *testing.T) {
	conn := mustUserDB(t)
	defer conn.Close()
	if _, err := NewUsers(conn).Create(context.Background(), 0, "a@x.com", "h", "N", "owner", false); err == nil {
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
