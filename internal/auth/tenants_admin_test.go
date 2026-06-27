package auth

import (
	"context"
	"database/sql"
	"testing"
)

// mustAdminUser provisions a platform-admin user in its own tenant and returns
// the admin's user id. The admin acts cross-tenant on OTHER tenants; its id is
// a real users(id) so the audit_log.user_id FK is satisfied.
func mustAdminUser(t *testing.T, conn *sql.DB) (adminTenantID, adminUserID string) {
	t.Helper()
	ctx := context.Background()
	adminTenant, err := NewTenants(conn).Create(ctx, "Platform Admin Tenant")
	if err != nil {
		t.Fatalf("create admin tenant: %v", err)
	}
	admin, err := NewUsers(conn).Create(ctx, adminTenant.ID,
		"admin@tallyo.test", "uid-admin", "Platform Admin", "owner", true)
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	return adminTenant.ID, admin.ID
}

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

// auditRowCount returns how many audit rows match the action + entity_id AND are
// stamped with the given target tenant_id and acting user_id. A NULL tenant_id
// is matched by passing wantTenantNull = true (the delete case).
func auditRowCount(t *testing.T, conn *sql.DB, action, entityID, wantTenantID, wantUserID string, wantTenantNull bool) int {
	t.Helper()
	var n int
	var err error
	if wantTenantNull {
		err = conn.QueryRow(
			`SELECT COUNT(*) FROM audit_log
			 WHERE entity_type='tenant' AND action=$1 AND entity_id=$2
			   AND tenant_id IS NULL AND user_id=$3`,
			action, entityID, wantUserID,
		).Scan(&n)
	} else {
		err = conn.QueryRow(
			`SELECT COUNT(*) FROM audit_log
			 WHERE entity_type='tenant' AND action=$1 AND entity_id=$2
			   AND tenant_id=$3 AND user_id=$4`,
			action, entityID, wantTenantID, wantUserID,
		).Scan(&n)
	}
	if err != nil {
		t.Fatalf("audit count (%s): %v", action, err)
	}
	return n
}

// TestTenantsSuspendAndUnsuspend verifies that Suspend sets the StatusSuspended
// status (which the login/ResolveTenant path blocks on) and Unsuspend restores
// StatusActive. Audit rows must carry the TARGET tenant and the ACTING admin.
func TestTenantsSuspendAndUnsuspend(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	_, adminID := mustAdminUser(t, conn)

	tn, err := repo.Create(ctx, "Suspend Me")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

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

	// Audit row must be stamped with the target tenant AND the acting admin.
	if n := auditRowCount(t, conn, "suspend", tn.ID, tn.ID, adminID, false); n != 1 {
		t.Errorf("suspend audit rows (tenant=%s,user=%s) = %d, want 1", tn.ID, adminID, n)
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

	if n := auditRowCount(t, conn, "unsuspend", tn.ID, tn.ID, adminID, false); n != 1 {
		t.Errorf("unsuspend audit rows (tenant=%s,user=%s) = %d, want 1", tn.ID, adminID, n)
	}
}

// TestTenantsDelete verifies that Delete removes the tenant (and its dependents)
// permanently and writes a delete-audit row. The delete audit row carries
// tenant_id = NULL (the tenant is gone) with the gone tenant in entity_id and
// the acting admin in user_id.
func TestTenantsDelete(t *testing.T) {
	conn := mustTenantDB(t)
	repo := NewTenants(conn)
	ctx := context.Background()

	_, adminID := mustAdminUser(t, conn)

	// Provision a full tenant (owner user + profile) so Delete must clean up
	// dependents, not just a bare tenant row.
	owner, err := repo.Signup(ctx, SignupInput{
		BusinessName: "Doomed Tenant", Email: "owner@doomed.test", FirebaseUID: "uid-doomed", OwnerName: "Doomed Owner",
	}, profileProv(conn))
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}
	tenantID := owner.TenantID

	if err := repo.Delete(ctx, tenantID, adminID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Tenant must be gone.
	_, found, err := repo.Status(ctx, tenantID)
	if err != nil {
		t.Fatalf("Status after Delete: %v", err)
	}
	if found {
		t.Error("tenant still exists after Delete")
	}

	// Its users must be gone too.
	var userRows int
	if err := conn.QueryRow("SELECT COUNT(*) FROM users WHERE tenant_id=$1", tenantID).Scan(&userRows); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userRows != 0 {
		t.Errorf("users for deleted tenant = %d, want 0", userRows)
	}

	// Delete audit row: tenant_id NULL, entity_id = gone tenant, user_id = admin.
	if n := auditRowCount(t, conn, "delete", tenantID, "", adminID, true); n != 1 {
		t.Errorf("delete audit rows (tenant NULL,user=%s) = %d, want 1", adminID, n)
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
