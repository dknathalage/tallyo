-- name: ListPayers :many
SELECT * FROM payers WHERE tenant_id = ? ORDER BY name;

-- name: SearchPayers :many
SELECT * FROM payers
WHERE tenant_id = ? AND (name LIKE ? OR email LIKE ?)
ORDER BY name;

-- name: GetPayer :one
SELECT * FROM payers WHERE tenant_id = ? AND uuid = ?;

-- name: GetPayerIDByUUID :one
SELECT id FROM payers WHERE tenant_id = ? AND uuid = ?;

-- name: GetPayerByID :one
SELECT * FROM payers WHERE tenant_id = ? AND id = ?;

-- name: CreatePayer :one
INSERT INTO payers (uuid, tenant_id, name, email, phone, address, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdatePayer :one
UPDATE payers SET name = ?, email = ?, phone = ?, address = ?, metadata = ?, updated_at = ?
WHERE tenant_id = ? AND uuid = ?
RETURNING *;

-- name: DeletePayer :exec
DELETE FROM payers WHERE tenant_id = ? AND uuid = ?;

-- name: DeletePayerByID :exec
DELETE FROM payers WHERE tenant_id = ? AND id = ?;
