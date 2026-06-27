-- name: CreateTenant :one
INSERT INTO tenants (id, name, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantByUUID :one
SELECT * FROM tenants WHERE id = $1;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY created_at DESC;

-- name: UpdateTenant :one
UPDATE tenants SET name = $1, updated_at = $2
WHERE id = $3
RETURNING *;

-- name: UpdateTenantStatus :exec
UPDATE tenants SET status = $1, updated_at = $2 WHERE id = $3;

-- name: DeleteTenant :exec
DELETE FROM tenants WHERE id = $1;
