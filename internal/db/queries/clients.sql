-- name: ListClients :many
SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ?
ORDER BY p.name;

-- name: SearchClients :many
SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ? AND (p.name LIKE ? OR p.email LIKE ? OR p.reference LIKE ?)
ORDER BY p.name;

-- name: GetClient :one
SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ? AND p.id = ?;

-- name: GetClientIDByUUID :one
SELECT id FROM clients WHERE tenant_id = ? AND id = ?;

-- name: GetClientByID :one
SELECT p.*, pm.name AS payer_name, pm.id AS payer_uuid
FROM clients p
LEFT JOIN payers pm ON p.payer_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ? AND p.id = ?;

-- name: CreateClient :one
INSERT INTO clients (
    id, tenant_id, name, reference, payer_id,
    email, phone, address, metadata, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateClient :one
UPDATE clients SET
    name = ?, reference = ?, payer_id = ?,
    email = ?, phone = ?, address = ?, metadata = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?
RETURNING *;

-- name: DeleteClient :exec
DELETE FROM clients WHERE tenant_id = ? AND id = ?;

-- name: DeleteClientByID :exec
DELETE FROM clients WHERE tenant_id = ? AND id = ?;
