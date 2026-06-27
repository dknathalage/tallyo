-- +goose Up
-- SaaS subscription state (DB-per-tenant control plane). One tenant = one Stripe
-- subscription, so the fields live on the tenants row rather than a join table.
-- Stripe owns the trial clock + dunning; webhooks sync these columns. The
-- entitlement gate reads subscription_status; the rest are display-only.
-- subscription_synced_at carries the last applied event timestamp so out-of-order
-- / duplicate webhook deliveries are idempotent (older events no-op).
ALTER TABLE tenants ADD COLUMN stripe_customer_id     TEXT;
ALTER TABLE tenants ADD COLUMN stripe_subscription_id TEXT;
ALTER TABLE tenants ADD COLUMN subscription_status    TEXT NOT NULL DEFAULT 'none';
ALTER TABLE tenants ADD COLUMN trial_end              TEXT;
ALTER TABLE tenants ADD COLUMN current_period_end     TEXT;
ALTER TABLE tenants ADD COLUMN subscription_synced_at TEXT;
CREATE INDEX idx_tenants_stripe_customer ON tenants (stripe_customer_id);

-- +goose Down
DROP INDEX idx_tenants_stripe_customer;
ALTER TABLE tenants DROP COLUMN subscription_synced_at;
ALTER TABLE tenants DROP COLUMN current_period_end;
ALTER TABLE tenants DROP COLUMN trial_end;
ALTER TABLE tenants DROP COLUMN subscription_status;
ALTER TABLE tenants DROP COLUMN stripe_subscription_id;
ALTER TABLE tenants DROP COLUMN stripe_customer_id;
