package subscription

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dknathalage/tallyo/internal/db/gen"
)

// Store reads and writes the subscription mirror columns on the control-DB
// tenants table. It is the only writer of those columns — the Stripe webhook
// calls Apply; ResolveTenant reads via auth.TenantsRepo.
type Store struct {
	db *sql.DB
}

// NewStore constructs a Store. A nil db is a programmer error.
func NewStore(db *sql.DB) *Store {
	if db == nil {
		panic("subscription: NewStore requires a non-nil *sql.DB")
	}
	return &Store{db: db}
}

// Update carries the subscription state to write for one tenant. All timestamps
// are RFC3339 UTC strings ("" → NULL). SyncedAt is the source event's timestamp
// and drives idempotency: an Update whose SyncedAt is not newer than what is
// stored is a no-op (handles Stripe's at-least-once, possibly-out-of-order
// delivery).
type Update struct {
	TenantID             string
	StripeCustomerID     string
	StripeSubscriptionID string
	Status               string
	TrialEnd             string
	CurrentPeriodEnd     string
	SyncedAt             string
}

// Apply writes the subscription state for u.TenantID unless the stored
// subscription_synced_at is already >= u.SyncedAt (stale/duplicate event). It
// reports whether a write happened.
func (s *Store) Apply(ctx context.Context, u Update) (applied bool, err error) {
	if u.TenantID == "" {
		return false, errors.New("subscription apply: tenant id required")
	}
	q := gen.New(s.db)
	cur, err := q.GetTenant(ctx, u.TenantID)
	if err != nil {
		return false, fmt.Errorf("subscription apply: load tenant: %w", err)
	}
	// RFC3339-UTC strings sort chronologically, so a lexicographic compare is a
	// valid "is this event newer?" check. Equal timestamps are treated as
	// duplicates → no-op.
	if u.SyncedAt != "" && cur.SubscriptionSyncedAt.Valid && u.SyncedAt <= cur.SubscriptionSyncedAt.String {
		return false, nil
	}
	err = q.UpdateTenantSubscription(ctx, gen.UpdateTenantSubscriptionParams{
		StripeCustomerID:     nullStr(u.StripeCustomerID),
		StripeSubscriptionID: nullStr(u.StripeSubscriptionID),
		SubscriptionStatus:   u.Status,
		TrialEnd:             nullStr(u.TrialEnd),
		CurrentPeriodEnd:     nullStr(u.CurrentPeriodEnd),
		SubscriptionSyncedAt: nullStr(u.SyncedAt),
		UpdatedAt:            time.Now().UTC().Format(time.RFC3339),
		ID:                   u.TenantID,
	})
	if err != nil {
		return false, fmt.Errorf("subscription apply: update: %w", err)
	}
	return true, nil
}

// GetTenantByStripeCustomer resolves the tenant id linked to a Stripe customer.
// Returns ("", false, nil) when no tenant carries that customer id (used by the
// webhook to map a subscription event back to a tenant).
func (s *Store) GetTenantByStripeCustomer(ctx context.Context, customerID string) (tenantID string, found bool, err error) {
	if customerID == "" {
		return "", false, errors.New("subscription lookup: customer id required")
	}
	row, qerr := gen.New(s.db).GetTenantByStripeCustomer(ctx, nullStr(customerID))
	if errors.Is(qerr, sql.ErrNoRows) {
		return "", false, nil
	}
	if qerr != nil {
		return "", false, fmt.Errorf("subscription lookup: %w", qerr)
	}
	return row.ID, true, nil
}

func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
