-- name: ListPayers :many
SELECT * FROM payers WHERE tenant_id = $1 ORDER BY name;

-- name: SearchPayers :many
SELECT * FROM payers
WHERE tenant_id = $1 AND (name LIKE $2 OR email LIKE $3)
ORDER BY name;

-- name: GetPayer :one
SELECT * FROM payers WHERE tenant_id = $1 AND id = $2;

-- name: GetPayerIDByUUID :one
SELECT id FROM payers WHERE tenant_id = $1 AND id = $2;

-- name: GetPayerByID :one
SELECT * FROM payers WHERE tenant_id = $1 AND id = $2;

-- name: CreatePayer :one
INSERT INTO payers (id, tenant_id, name, email, phone, address, metadata, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdatePayer :one
UPDATE payers SET name = $1, email = $2, phone = $3, address = $4, metadata = $5, updated_at = $6
WHERE tenant_id = $7 AND id = $8
RETURNING *;

-- name: DeletePayer :exec
DELETE FROM payers WHERE tenant_id = $1 AND id = $2;

-- name: DeletePayerByID :exec
DELETE FROM payers WHERE tenant_id = $1 AND id = $2;
