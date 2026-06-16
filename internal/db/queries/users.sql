-- name: CountUsers :one
SELECT COUNT(*) FROM users WHERE tenant_id = ?;

-- name: CreateUser :one
INSERT INTO users (uuid, tenant_id, email, password_hash, name, is_platform_admin, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE tenant_id = ? AND email = ?;

-- name: GetUserByEmailGlobal :one
SELECT * FROM users WHERE email = ?;

-- name: CountUsersByEmailGlobal :one
SELECT COUNT(*) FROM users WHERE email = ?;

-- name: ListTenantsByEmail :many
SELECT u.tenant_id, t.name AS tenant_name, t.uuid AS tenant_uuid
FROM users u
JOIN tenants t ON t.id = u.tenant_id
WHERE u.email = ?
ORDER BY t.name;

-- name: GetUserByID :one
SELECT * FROM users WHERE tenant_id = ? AND id = ?;

-- name: ListUsers :many
SELECT * FROM users WHERE tenant_id = ? ORDER BY id;

-- name: UpdateUserRole :exec
UPDATE users SET role = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: DeleteUser :exec
DELETE FROM users WHERE tenant_id = ? AND id = ?;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = ? WHERE id = ?;
