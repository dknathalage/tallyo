package subscription

import (
	"context"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

// TestSetSubscriptionStatusWritesStatusAndAudit verifies the happy path:
// - subscription_status is updated
// - Stripe customer/subscription IDs are left untouched
// - an audit row is written attributed to the target tenant
func TestSetSubscriptionStatusWritesStatusAndAudit(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()

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

	const adminID = "admin-uuid-001"

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

	// Audit row must be present.
	var auditRows int
	if err := conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='tenant' AND action='set_subscription_status' AND entity_id=$1",
		tenant.ID,
	).Scan(&auditRows); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditRows != 1 {
		t.Errorf("audit rows = %d, want 1", auditRows)
	}
}

// TestSetSubscriptionStatusTrialingWritesTrialEnd ensures that when status is
// trialing and a trialEndsAt date is supplied, trial_end is written.
func TestSetSubscriptionStatusTrialingWritesTrialEnd(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()

	tenant, err := auth.NewTenants(conn).Create(ctx, "Trial Extender")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	store := NewStore(conn)
	const trialEnd = "2026-12-31T00:00:00Z"

	if err := store.SetSubscriptionStatus(ctx, tenant.ID, StatusTrialing, "admin-uuid", trialEnd); err != nil {
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
