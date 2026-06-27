-- name: ListClients :many
SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = $1
ORDER BY p.name;

-- name: SearchClients :many
SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = $1 AND (p.name LIKE $2 OR p.email LIKE $3 OR p.reference LIKE $4)
ORDER BY p.name;

-- name: GetClient :one
SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = $1 AND p.id = $2;

-- name: GetClientIDByUUID :one
SELECT id FROM clients WHERE tenant_id = $1 AND id = $2;

-- name: GetClientByID :one
SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = $1 AND p.id = $2;

-- name: CreateClient :one
INSERT INTO clients (
    id, tenant_id, name, reference, payer_id,
    email, phone, address, metadata, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateClient :one
UPDATE clients SET
    name = $1, reference = $2, payer_id = $3,
    email = $4, phone = $5, address = $6, metadata = $7, updated_at = $8
WHERE tenant_id = $9 AND id = $10
RETURNING *;

-- name: DeleteClient :exec
DELETE FROM clients WHERE tenant_id = $1 AND id = $2;

-- name: DeleteClientByID :exec
DELETE FROM clients WHERE tenant_id = $1 AND id = $2;
