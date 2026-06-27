package auth

import (
	"context"
	"testing"
)

// TestTenantsListReturnsUserCounts ensures List includes all tenants and their
// per-tenant user counts.
func TestTenantsListReturnsUserCounts(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	// Seed two tenants via Signup so each gets an owner user.
	ownerA, err := repo.Signup(ctx, SignupInput{
		BusinessName: "Alpha Co", Email: "a@alpha.com", FirebaseUID: "uid-a", OwnerName: "A",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("signup alpha: %v", err)
	}
	_, err = repo.Signup(ctx, SignupInput{
		BusinessName: "Beta Co", Email: "b@beta.com", FirebaseUID: "uid-b", OwnerName: "B",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("signup beta: %v", err)
	}

	// Add a second user to Alpha.
	usersRepo := NewUsers(conn)
	_, err = usersRepo.Create(ctx, ownerA.TenantID, "extra@alpha.com", "uid-a2", "Extra", "member", false)
	if err != nil {
		t.Fatalf("create extra user: %v", err)
	}

	summaries, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("List returned %d tenants, want 2", len(summaries))
	}

	// Build a map for easier lookup.
	byName := make(map[string]*TenantSummary, len(summaries))
	for _, s := range summaries {
		byName[s.Name] = s
	}

	alpha, ok := byName["Alpha Co"]
	if !ok {
		t.Fatal("List: Alpha Co not found")
	}
	if alpha.UserCount != 2 {
		t.Errorf("Alpha Co user count = %d, want 2", alpha.UserCount)
	}

	beta, ok := byName["Beta Co"]
	if !ok {
		t.Fatal("List: Beta Co not found")
	}
	if beta.UserCount != 1 {
		t.Errorf("Beta Co user count = %d, want 1", beta.UserCount)
	}
}

// TestTenantsSuspendAndUnsuspend verifies that Suspend sets the StatusSuspended
// status (which the login/ResolveTenant path blocks on) and Unsuspend restores
// StatusActive.
func TestTenantsSuspendAndUnsuspend(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "Suspend Me")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const adminID = "admin-user-uuid"

	// Suspend.
	if err := repo.Suspend(ctx, tn.ID, adminID); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	status, found, err := repo.Status(ctx, tn.ID)
	if err != nil || !found {
		t.Fatalf("Status after Suspend: status=%q found=%v err=%v", status, found, err)
	}
	if status != StatusSuspended {
		t.Errorf("status after Suspend = %q, want %q", status, StatusSuspended)
	}

	// Audit row must exist.
	var rows int
	if err := conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='tenant' AND action='suspend' AND entity_id=$1", tn.ID,
	).Scan(&rows); err != nil {
		t.Fatalf("audit suspend: %v", err)
	}
	if rows != 1 {
		t.Errorf("suspend audit rows = %d, want 1", rows)
	}

	// Unsuspend.
	if err := repo.Unsuspend(ctx, tn.ID, adminID); err != nil {
		t.Fatalf("Unsuspend: %v", err)
	}

	status, found, err = repo.Status(ctx, tn.ID)
	if err != nil || !found {
		t.Fatalf("Status after Unsuspend: status=%q found=%v err=%v", status, found, err)
	}
	if status != StatusActive {
		t.Errorf("status after Unsuspend = %q, want %q", status, StatusActive)
	}

	// Audit row for unsuspend.
	if err := conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='tenant' AND action='unsuspend' AND entity_id=$1", tn.ID,
	).Scan(&rows); err != nil {
		t.Fatalf("audit unsuspend: %v", err)
	}
	if rows != 1 {
		t.Errorf("unsuspend audit rows = %d, want 1", rows)
	}
}

// TestTenantsDelete verifies that Delete removes the tenant permanently.
func TestTenantsDelete(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	tn, err := repo.Create(ctx, "Doomed Tenant")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const adminID = "admin-user-uuid"
	if err := repo.Delete(ctx, tn.ID, adminID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Tenant must be gone.
	_, found, err := repo.Status(ctx, tn.ID)
	if err != nil {
		t.Fatalf("Status after Delete: %v", err)
	}
	if found {
		t.Error("tenant still exists after Delete")
	}

	// Audit row must exist (written before the delete).
	var rows int
	if err := conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='tenant' AND action='delete' AND entity_id=$1", tn.ID,
	).Scan(&rows); err != nil {
		t.Fatalf("audit delete: %v", err)
	}
	if rows != 1 {
		t.Errorf("delete audit rows = %d, want 1", rows)
	}
}

// TestTenantsSuspendRejectsEmptyUUID checks the guard on empty tenantUUID.
func TestTenantsSuspendRejectsEmptyUUID(t *testing.T) {
	conn := mustTenantDB(t)
	if err := NewTenants(conn).Suspend(context.Background(), "", "admin"); err == nil {
		t.Fatal("Suspend with empty uuid must error")
	}
}

// TestTenantsUnsuspendRejectsEmptyUUID checks the guard on empty tenantUUID.
func TestTenantsUnsuspendRejectsEmptyUUID(t *testing.T) {
	conn := mustTenantDB(t)
	if err := NewTenants(conn).Unsuspend(context.Background(), "", "admin"); err == nil {
		t.Fatal("Unsuspend with empty uuid must error")
	}
}

// TestTenantsDeleteRejectsEmptyUUID checks the guard on empty tenantUUID.
func TestTenantsDeleteRejectsEmptyUUID(t *testing.T) {
	conn := mustTenantDB(t)
	if err := NewTenants(conn).Delete(context.Background(), "", "admin"); err == nil {
		t.Fatal("Delete with empty uuid must error")
	}
}
