package subscription

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// mustAdminUser provisions a platform-admin user in its own tenant and returns
// the admin's user id. The admin id is a real users(id) so the audit_log
// user_id FK is satisfied when SetSubscriptionStatus stamps the audit row.
func mustAdminUser(t *testing.T, conn *sql.DB) string {
	t.Helper()
	ctx := context.Background()
	adminTenant, err := auth.NewTenants(conn).Create(ctx, "Platform Admin Tenant")
	if err != nil {
		t.Fatalf("create admin tenant: %v", err)
	}
	admin, err := auth.NewUsers(conn).Create(ctx, adminTenant.ID,
		"admin@tallyo.test", "uid-admin", "Platform Admin", "owner", true)
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	return admin.ID
}

// TestSetSubscriptionStatusWritesStatusAndAudit verifies the happy path:
// - subscription_status is updated
// - Stripe customer/subscription IDs are left untouched
// - an audit row is written attributed to the target tenant and acting admin
func TestSetSubscriptionStatusWritesStatusAndAudit(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()

	adminID := mustAdminUser(t, conn)

	tenant, err := auth.NewTenants(conn).Create(ctx, "Override Me")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	store := NewStore(conn)

	// First, set Stripe IDs via Apply so we can verify they survive the override.
	if _, err := store.Apply(ctx, Update{
		TenantID:             tenant.ID,
		StripeCustomerID:     "cus_admin_test",
		StripeSubscriptionID: "sub_admin_test",
		Status:               StatusTrialing,
		SyncedAt:             "2026-06-01T10:00:00Z",
	}); err != nil {
		t.Fatalf("Apply (seed Stripe IDs): %v", err)
	}

	// Override to active.
	if err := store.SetSubscriptionStatus(ctx, tenant.ID, StatusActive, adminID, ""); err != nil {
		t.Fatalf("SetSubscriptionStatus: %v", err)
	}

	row, err := gen.New(conn).GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}

	// Status must be updated.
	if row.SubscriptionStatus != StatusActive {
		t.Errorf("subscription_status = %q, want %q", row.SubscriptionStatus, StatusActive)
	}

	// Stripe IDs must be untouched.
	if row.StripeCustomerID.String != "cus_admin_test" {
		t.Errorf("stripe_customer_id = %q, want %q", row.StripeCustomerID.String, "cus_admin_test")
	}
	if row.StripeSubscriptionID.String != "sub_admin_test" {
		t.Errorf("stripe_subscription_id = %q, want %q", row.StripeSubscriptionID.String, "sub_admin_test")
	}

	// Audit row must be stamped with the TARGET tenant AND the ACTING admin.
	var auditRows int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log
		 WHERE entity_type='tenant' AND action='set_subscription_status'
		   AND entity_id=$1 AND tenant_id=$1 AND user_id=$2`,
		tenant.ID, adminID,
	).Scan(&auditRows); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditRows != 1 {
		t.Errorf("audit rows (tenant=%s,user=%s) = %d, want 1", tenant.ID, adminID, auditRows)
	}
}

// TestSetSubscriptionStatusTrialingWritesTrialEnd ensures that when status is
// trialing and a trialEndsAt date is supplied, trial_end is written.
func TestSetSubscriptionStatusTrialingWritesTrialEnd(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()

	adminID := mustAdminUser(t, conn)

	tenant, err := auth.NewTenants(conn).Create(ctx, "Trial Extender")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	store := NewStore(conn)
	const trialEnd = "2026-12-31T00:00:00Z"

	if err := store.SetSubscriptionStatus(ctx, tenant.ID, StatusTrialing, adminID, trialEnd); err != nil {
		t.Fatalf("SetSubscriptionStatus trialing: %v", err)
	}

	row, err := gen.New(conn).GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if row.SubscriptionStatus != StatusTrialing {
		t.Errorf("status = %q, want %q", row.SubscriptionStatus, StatusTrialing)
	}
	if !row.TrialEnd.Valid || row.TrialEnd.String != trialEnd {
		t.Errorf("trial_end = %v/%q, want valid %q", row.TrialEnd.Valid, row.TrialEnd.String, trialEnd)
	}
}

// TestSetSubscriptionStatusNonTrialingClearsTrialEnd verifies that overriding a
// trialing tenant (with a trial_end set) to a non-trialing status CLEARS
// trial_end to NULL — the column is always written.
func TestSetSubscriptionStatusNonTrialingClearsTrialEnd(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()

	adminID := mustAdminUser(t, conn)

	tenant, err := auth.NewTenants(conn).Create(ctx, "Trial Then Active")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	store := NewStore(conn)
	const trialEnd = "2026-12-31T00:00:00Z"

	// Set trialing with a trial_end.
	if err := store.SetSubscriptionStatus(ctx, tenant.ID, StatusTrialing, adminID, trialEnd); err != nil {
		t.Fatalf("SetSubscriptionStatus trialing: %v", err)
	}
	row, err := gen.New(conn).GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant after trialing: %v", err)
	}
	if !row.TrialEnd.Valid {
		t.Fatal("precondition: trial_end should be set after trialing override")
	}

	// Override to active — trial_end must be cleared.
	if err := store.SetSubscriptionStatus(ctx, tenant.ID, StatusActive, adminID, ""); err != nil {
		t.Fatalf("SetSubscriptionStatus active: %v", err)
	}
	row, err = gen.New(conn).GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant after active: %v", err)
	}
	if row.SubscriptionStatus != StatusActive {
		t.Errorf("status = %q, want %q", row.SubscriptionStatus, StatusActive)
	}
	if row.TrialEnd.Valid {
		t.Errorf("trial_end = %q, want NULL after non-trialing override", row.TrialEnd.String)
	}
}

// TestSetSubscriptionStatusRejectsInvalidStatus verifies that unknown statuses
// are rejected before any DB write.
func TestSetSubscriptionStatusRejectsInvalidStatus(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()

	tenant, err := auth.NewTenants(conn).Create(ctx, "Guard Me")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	store := NewStore(conn)
	invalidStatuses := []string{"comp", "unknown", "", "ACTIVE", "suspended"}
	for _, s := range invalidStatuses {
		if err := store.SetSubscriptionStatus(ctx, tenant.ID, s, "admin", ""); err == nil {
			t.Errorf("SetSubscriptionStatus(%q) expected error, got nil", s)
		}
	}
}

// TestSetSubscriptionStatusRejectsEmptyTenantID verifies the guard.
func TestSetSubscriptionStatusRejectsEmptyTenantID(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	if err := NewStore(conn).SetSubscriptionStatus(context.Background(), "", StatusActive, "admin", ""); err == nil {
		t.Fatal("empty tenant id must error")
	}
}
