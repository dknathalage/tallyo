-- name: CountUsers :one
SELECT COUNT(*) FROM users WHERE tenant_id = $1;

-- name: CreateUser :one
INSERT INTO users (id, tenant_id, email, password_hash, name, is_platform_admin, role, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE tenant_id = $1 AND email = $2;

-- name: GetUserByEmailGlobal :one
SELECT * FROM users WHERE email = $1;

-- name: CountUsersByEmailGlobal :one
SELECT COUNT(*) FROM users WHERE email = $1;

-- name: ListTenantsByEmail :many
SELECT u.tenant_id, t.name AS tenant_name, t.id AS tenant_uuid, u.role AS role
FROM users u
JOIN tenants t ON t.id = u.tenant_id
WHERE u.email = $1
ORDER BY t.name;

-- name: GetUserByID :one
SELECT * FROM users WHERE tenant_id = $1 AND id = $2;

-- name: ListUsers :many
SELECT * FROM users WHERE tenant_id = $1 ORDER BY id;

-- name: UpdateUserRole :exec
UPDATE users SET role = $1, updated_at = $2 WHERE tenant_id = $3 AND id = $4;

-- name: DeleteUser :exec
DELETE FROM users WHERE tenant_id = $1 AND id = $2;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = $1 WHERE id = $2;
