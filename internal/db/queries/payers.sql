-- name: ListPayers :many
SELECT * FROM payers ORDER BY name;

-- name: SearchPayers :many
SELECT * FROM payers WHERE name LIKE ? OR email LIKE ? ORDER BY name;

-- name: GetPayer :one
SELECT * FROM payers WHERE id = ?;

-- name: CreatePayer :one
INSERT INTO payers (uuid, name, email, phone, address, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdatePayer :one
UPDATE payers SET name = ?, email = ?, phone = ?, address = ?, metadata = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeletePayer :exec
DELETE FROM payers WHERE id = ?;
