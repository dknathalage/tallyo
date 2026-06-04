-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CreateUser :one
INSERT INTO users (uuid, email, password_hash, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ?;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ?;

-- name: ListUsers :many
SELECT * FROM users ORDER BY id;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = ? WHERE id = ?;
