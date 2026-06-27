package subscription

import (
	"context"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/db/gen"
)

func TestStoreApplyAndLookup(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()
	tenant, err := auth.NewTenants(conn).Create(ctx, "Acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	store := NewStore(conn)

	// First apply links the customer and sets trialing.
	applied, err := store.Apply(ctx, Update{
		TenantID:             tenant.ID,
		StripeCustomerID:     "cus_1",
		StripeSubscriptionID: "sub_1",
		Status:               StatusTrialing,
		TrialEnd:             "2026-09-25T00:00:00Z",
		CurrentPeriodEnd:     "2026-09-25T00:00:00Z",
		SyncedAt:             "2026-06-27T10:00:00Z",
	})
	if err != nil || !applied {
		t.Fatalf("first Apply = (%v,%v), want (true,nil)", applied, err)
	}

	// Reverse lookup finds the tenant by customer id.
	gotID, found, err := store.GetTenantByStripeCustomer(ctx, "cus_1")
	if err != nil || !found || gotID != tenant.ID {
		t.Fatalf("GetTenantByStripeCustomer = (%q,%v,%v), want (%q,true,nil)", gotID, found, err, tenant.ID)
	}

	// Verify the row.
	row, err := gen.New(conn).GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if row.SubscriptionStatus != StatusTrialing {
		t.Errorf("status = %q, want %q", row.SubscriptionStatus, StatusTrialing)
	}

	// A newer event transitions to active.
	applied, err = store.Apply(ctx, Update{
		TenantID:             tenant.ID,
		StripeCustomerID:     "cus_1",
		StripeSubscriptionID: "sub_1",
		Status:               StatusActive,
		SyncedAt:             "2026-09-25T10:00:00Z",
	})
	if err != nil || !applied {
		t.Fatalf("newer Apply = (%v,%v), want (true,nil)", applied, err)
	}

	// A stale (older) event is a no-op and must not clobber active.
	applied, err = store.Apply(ctx, Update{
		TenantID:         tenant.ID,
		StripeCustomerID: "cus_1",
		Status:           StatusPastDue,
		SyncedAt:         "2026-07-01T10:00:00Z", // older than 09-25
	})
	if err != nil {
		t.Fatalf("stale Apply err: %v", err)
	}
	if applied {
		t.Error("stale Apply should be a no-op")
	}
	row, _ = gen.New(conn).GetTenant(ctx, tenant.ID)
	if row.SubscriptionStatus != StatusActive {
		t.Errorf("after stale event status = %q, want %q (no clobber)", row.SubscriptionStatus, StatusActive)
	}

	// Duplicate of the active event (equal timestamp) is also a no-op.
	applied, _ = store.Apply(ctx, Update{
		TenantID: tenant.ID, StripeCustomerID: "cus_1", Status: StatusActive,
		SyncedAt: "2026-09-25T10:00:00Z",
	})
	if applied {
		t.Error("duplicate event (equal timestamp) should be a no-op")
	}
}

func TestStoreLookupMissing(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	_, found, err := NewStore(conn).GetTenantByStripeCustomer(context.Background(), "cus_nope")
	if err != nil || found {
		t.Fatalf("lookup missing = (found=%v, err=%v), want (false, nil)", found, err)
	}
}
