-- name: CreateTenant :one
INSERT INTO tenants (uuid, name, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = ?;

-- name: GetTenantByUUID :one
SELECT * FROM tenants WHERE uuid = ?;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY created_at DESC;

-- name: UpdateTenant :one
UPDATE tenants SET name = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateTenantStatus :exec
UPDATE tenants SET status = ?, updated_at = ? WHERE id = ?;

-- name: DeleteTenant :exec
DELETE FROM tenants WHERE id = ?;
