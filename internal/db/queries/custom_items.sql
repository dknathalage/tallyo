-- name: ListCustomItems :many
SELECT * FROM custom_items WHERE tenant_id = ? ORDER BY name;

-- name: SearchCustomItems :many
SELECT * FROM custom_items WHERE tenant_id = ? AND name LIKE ? ORDER BY name;

-- name: GetCustomItem :one
SELECT * FROM custom_items WHERE tenant_id = ? AND uuid = ?;

-- name: GetCustomItemIDByUUID :one
SELECT id FROM custom_items WHERE tenant_id = ? AND uuid = ?;

-- name: CreateCustomItem :one
INSERT INTO custom_items (uuid, tenant_id, name, rate, unit, taxable, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCustomItem :one
UPDATE custom_items SET name = ?, rate = ?, unit = ?, taxable = ?, metadata = ?, updated_at = ?
WHERE tenant_id = ? AND uuid = ?
RETURNING *;

-- name: DeleteCustomItem :exec
DELETE FROM custom_items WHERE tenant_id = ? AND uuid = ?;

-- name: DeleteCustomItemByID :exec
DELETE FROM custom_items WHERE tenant_id = ? AND id = ?;
