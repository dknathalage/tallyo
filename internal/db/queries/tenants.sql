-- name: CreateTenant :one
INSERT INTO tenants (id, name, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantByUUID :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantByStripeCustomer :one
SELECT * FROM tenants WHERE stripe_customer_id = $1;

-- name: UpdateTenantSubscription :exec
UPDATE tenants SET
    stripe_customer_id     = $1,
    stripe_subscription_id = $2,
    subscription_status    = $3,
    trial_end              = $4,
    current_period_end     = $5,
    subscription_synced_at = $6,
    updated_at             = $7
WHERE id = $8;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY created_at DESC;

-- name: UpdateTenant :one
UPDATE tenants SET name = $1, updated_at = $2
WHERE id = $3
RETURNING *;

-- name: UpdateTenantStatus :execrows
-- :execrows so callers can detect a no-match (unknown tenant id) and 404 rather
-- than silently succeeding.
UPDATE tenants SET status = $1, updated_at = $2 WHERE id = $3;

-- name: DeleteTenant :exec
DELETE FROM tenants WHERE id = $1;

-- Low-frequency platform-admin query: full tenants scan + user-count join, not
-- a hot path. Fine to leave unindexed at expected tenant counts.
-- name: ListTenantsWithUserCount :many
SELECT
    t.id,
    t.name,
    t.status,
    t.created_at,
    t.updated_at,
    t.stripe_customer_id,
    t.stripe_subscription_id,
    t.subscription_status,
    t.trial_end,
    t.current_period_end,
    t.subscription_synced_at,
    COUNT(u.id) AS user_count
FROM tenants t
LEFT JOIN users u ON u.tenant_id = t.id
GROUP BY t.id
ORDER BY t.created_at DESC;

-- name: SetTenantSubscriptionStatus :execrows
-- :execrows so an admin override on an unknown tenant id is detectable (404)
-- instead of a silent no-op.
UPDATE tenants
SET subscription_status = $1,
    trial_end            = $2,
    updated_at           = $3
WHERE id = $4;
