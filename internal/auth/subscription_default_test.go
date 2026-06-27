package auth

import (
	"context"
	"testing"
)

// TestNewTenantStartsWithNoneStatus locks in the migration default: a freshly
// created tenant has subscription_status "none" (signed up, not yet subscribed),
// so the billing gate treats it as not-entitled until Checkout completes.
func TestNewTenantStartsWithNoneStatus(t *testing.T) {
	conn := mustTenantDB(t)
	ctx := context.Background()
	tenant, err := NewTenants(conn).Create(ctx, "Acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	got, err := NewTenants(conn).GetByUUID(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetByUUID: %v", err)
	}
	if got.SubscriptionStatus != "none" {
		t.Errorf("SubscriptionStatus = %q, want %q", got.SubscriptionStatus, "none")
	}
}
