package auth

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// mustTenantDB returns a migrated, empty DB for tenant-level tests.
func mustTenantDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}

func TestTenantCountAndCreate(t *testing.T) {
	conn := mustTenantDB(t)
	defer conn.Close()
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
	if tn.ID == 0 || tn.UUID == "" {
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
	defer conn.Close()
	if _, err := NewTenants(conn).Create(context.Background(), ""); err == nil {
		t.Fatal("empty name must error")
	}
}

func TestTenantStatus(t *testing.T) {
	conn := mustTenantDB(t)
	defer conn.Close()
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
	defer conn.Close()
	status, found, err := NewTenants(conn).Status(context.Background(), 99999)
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
	defer conn.Close()
	if _, _, err := NewTenants(conn).Status(context.Background(), 0); err == nil {
		t.Fatal("zero tenant id must error")
	}
}

func TestSignupProvisionsTenantOwnerAndProfile(t *testing.T) {
	conn := mustTenantDB(t)
	defer conn.Close()
	repo := NewTenants(conn)
	ctx := context.Background()

	hash, err := HashPassword("pw123456")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	owner, err := repo.Signup(ctx, SignupInput{
		BusinessName: "Signup Co",
		Email:        "owner@signup.com",
		PasswordHash: hash,
		OwnerName:    "Owner Person",
		Zone:         "remote",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}
	if owner == nil || owner.Email != "owner@signup.com" || owner.Role != "owner" {
		t.Fatalf("bad owner %+v", owner)
	}
	if owner.TenantID == 0 {
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

	// the business_profile must carry the requested zone
	prof, err := gen.New(conn).GetBusinessProfile(ctx, owner.TenantID)
	if err != nil {
		t.Fatalf("GetBusinessProfile: %v", err)
	}
	if prof.Zone != "remote" {
		t.Fatalf("profile zone=%q want remote", prof.Zone)
	}
	if prof.Name != "Signup Co" {
		t.Fatalf("profile name=%q want Signup Co", prof.Name)
	}

	// the owner is reachable via the global login lookup
	got, err := NewUsers(conn).GetByEmailGlobal(ctx, "owner@signup.com")
	if err != nil || got == nil {
		t.Fatalf("GetByEmailGlobal owner=%+v err=%v", got, err)
	}
}

// TestSignupEmptyZoneCreatesGenericTenant confirms a signup with no zone
// provisions a generic (non-NDIS) tenant whose profile zone is "" — not coerced
// to national.
func TestSignupEmptyZoneCreatesGenericTenant(t *testing.T) {
	conn := mustTenantDB(t)
	defer conn.Close()
	repo := NewTenants(conn)
	ctx := context.Background()

	hash, _ := HashPassword("pw123456")
	owner, err := repo.Signup(ctx, SignupInput{
		BusinessName: "NoZone Co",
		Email:        "nozone@x.com",
		PasswordHash: hash,
		OwnerName:    "Owner",
		// Zone intentionally empty
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}
	prof, err := gen.New(conn).GetBusinessProfile(ctx, owner.TenantID)
	if err != nil {
		t.Fatalf("GetBusinessProfile: %v", err)
	}
	if prof.Zone != "" {
		t.Fatalf("generic tenant zone=%q want \"\"", prof.Zone)
	}
}

func TestSignupRejectsMissingRequiredFields(t *testing.T) {
	conn := mustTenantDB(t)
	defer conn.Close()
	repo := NewTenants(conn)
	ctx := context.Background()

	cases := []struct {
		name string
		in   SignupInput
	}{
		{"no business name", SignupInput{Email: "a@x.com", PasswordHash: "h"}},
		{"no email", SignupInput{BusinessName: "B", PasswordHash: "h"}},
		{"no password hash", SignupInput{BusinessName: "B", Email: "a@x.com"}},
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
	defer conn.Close()
	repo := NewTenants(conn)
	ctx := context.Background()

	hash, _ := HashPassword("pw123456")
	a, err := repo.Signup(ctx, SignupInput{
		BusinessName: "First", Email: "dup@x.com", PasswordHash: hash, OwnerName: "A", Zone: "remote",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("first signup: %v", err)
	}
	// Email is unique per-tenant (not global): a second signup creates a NEW
	// tenant, so the same email is allowed and a second tenant exists.
	b, err := repo.Signup(ctx, SignupInput{
		BusinessName: "Second", Email: "dup@x.com", PasswordHash: hash, OwnerName: "B", Zone: "remote",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("second signup: %v", err)
	}
	if a.TenantID == b.TenantID {
		t.Fatalf("both signups landed in tenant %d", a.TenantID)
	}

	n, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("tenant Count=%d want 2", n)
	}

	// The shared email is now ambiguous for global credential lookup (fail safe).
	if _, _, err := NewUsers(conn).GetCredentialsGlobal(ctx, "dup@x.com"); err != ErrAmbiguousEmail {
		t.Fatalf("GetCredentialsGlobal err=%v want ErrAmbiguousEmail", err)
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
	return func(ctx context.Context, tenantID int64, in SignupInput) error {
		return ProvisionBusinessProfile(ctx, conn, tenantID, in)
	}
}
