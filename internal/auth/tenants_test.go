package auth

import (
	"context"
	"database/sql"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// mustTenantDB returns a migrated, empty DB for tenant-level tests.
func mustTenantDB(t *testing.T) *sql.DB {
	t.Helper()
	conn := appdb.OpenTestDB(t)
	return conn
}

func TestTenantCountAndCreate(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	n, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Fatalf("initial Count=%d want 0", n)
	}

	tn, err := repo.Create(ctx, "Acme Pty Ltd")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tn.ID == "" {
		t.Fatalf("create returned bad ids %+v", tn)
	}
	if tn.Name != "Acme Pty Ltd" || tn.Status != "active" {
		t.Fatalf("create returned bad fields %+v", tn)
	}

	n, err = repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count after create: %v", err)
	}
	if n != 1 {
		t.Fatalf("Count after create=%d want 1", n)
	}

	// the mutation must be audited
	var rows int
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='tenant' AND action='create'").Scan(&rows); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("tenant create audit rows=%d want 1", rows)
	}
}

func TestTenantCreateRejectsEmptyName(t *testing.T) {
	conn := mustTenantDB(t)
	if _, err := NewTenants(conn).Create(context.Background(), ""); err == nil {
		t.Fatal("empty name must error")
	}
}

func TestTenantStatus(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "Acme")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	status, found, err := repo.Status(ctx, tn.ID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !found {
		t.Fatal("Status: found=false for existing tenant")
	}
	if status != StatusActive {
		t.Fatalf("Status=%q want %q", status, StatusActive)
	}
}

func TestTenantStatusMissingReturnsNotFound(t *testing.T) {
	conn := mustTenantDB(t)
	status, found, err := NewTenants(conn).Status(context.Background(), "no-such-tenant")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if found {
		t.Fatal("Status: found=true for unknown tenant")
	}
	if status != "" {
		t.Fatalf("Status=%q want empty for unknown tenant", status)
	}
}

func TestTenantStatusRejectsZeroID(t *testing.T) {
	conn := mustTenantDB(t)
	if _, _, err := NewTenants(conn).Status(context.Background(), ""); err == nil {
		t.Fatal("empty tenant id must error")
	}
}

func TestSignupProvisionsTenantOwnerAndProfile(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	owner, err := repo.Signup(ctx, SignupInput{
		BusinessName: "Signup Co",
		Email:        "owner@signup.com",
		FirebaseUID:  "uid-owner",
		OwnerName:    "Owner Person",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}
	if owner == nil || owner.Email != "owner@signup.com" || owner.Role != "owner" {
		t.Fatalf("bad owner %+v", owner)
	}
	if owner.TenantID == "" {
		t.Fatal("owner has no tenant id")
	}

	// exactly one tenant now exists
	n, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 1 {
		t.Fatalf("tenant Count=%d want 1", n)
	}

	// the business_profile must be provisioned for the new tenant
	prof, err := gen.New(conn).GetBusinessProfile(ctx, owner.TenantID)
	if err != nil {
		t.Fatalf("GetBusinessProfile: %v", err)
	}
	if prof.Name != "Signup Co" {
		t.Fatalf("profile name=%q want Signup Co", prof.Name)
	}

	// the owner is reachable via the tenant-scoped firebase-uid lookup
	got, err := NewUsers(conn).GetByFirebaseUID(ctx, owner.TenantID, "uid-owner")
	if err != nil || got == nil {
		t.Fatalf("GetByFirebaseUID owner=%+v err=%v", got, err)
	}
}

func TestSignupRejectsMissingRequiredFields(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	cases := []struct {
		name string
		in   SignupInput
	}{
		{"no business name", SignupInput{Email: "a@x.com", FirebaseUID: "uid"}},
		{"no email", SignupInput{BusinessName: "B", FirebaseUID: "uid"}},
		{"no firebase uid", SignupInput{BusinessName: "B", Email: "a@x.com"}},
	}
	// bounded loop: fixed-size table
	for _, tc := range cases {
		if _, err := repo.Signup(ctx, tc.in, profileProv(conn)); err == nil {
			t.Fatalf("%s: expected error, got nil", tc.name)
		}
	}
}

func TestSignupSameEmailDistinctTenants(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	a, err := repo.Signup(ctx, SignupInput{
		BusinessName: "First", Email: "dup@x.com", FirebaseUID: "uid-a", OwnerName: "A",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("first signup: %v", err)
	}
	// Email is unique per-tenant (not global): a second signup creates a NEW
	// tenant, so the same email is allowed and a second tenant exists.
	b, err := repo.Signup(ctx, SignupInput{
		BusinessName: "Second", Email: "dup@x.com", FirebaseUID: "uid-b", OwnerName: "B",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("second signup: %v", err)
	}
	if a.TenantID == b.TenantID {
		t.Fatalf("both signups landed in tenant %s", a.TenantID)
	}

	n, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("tenant Count=%d want 2", n)
	}
}

func TestNewTenantsNilDBPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewTenants(nil) must panic")
		}
	}()
	NewTenants(nil)
}

// profileProv is the single-DB test provisioner: it upserts the business_profile
// on the same handle the control rows were created on.
func profileProv(conn *sql.DB) ProfileProvisioner {
	return func(ctx context.Context, tenantID string, in SignupInput) error {
		return ProvisionBusinessProfile(ctx, conn, tenantID, in)
	}
}
