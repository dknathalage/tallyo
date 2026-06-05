-- name: ListClients :many
SELECT c.*, rt.name AS pricing_tier_name, p.name AS payer_name
FROM clients c
LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id
LEFT JOIN payers p ON c.payer_id = p.id
ORDER BY c.name;

-- name: SearchClients :many
SELECT c.*, rt.name AS pricing_tier_name, p.name AS payer_name
FROM clients c
LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id
LEFT JOIN payers p ON c.payer_id = p.id
WHERE c.name LIKE ? OR c.email LIKE ?
ORDER BY c.name;

-- name: GetClient :one
SELECT c.*, rt.name AS pricing_tier_name, p.name AS payer_name
FROM clients c
LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id
LEFT JOIN payers p ON c.payer_id = p.id
WHERE c.id = ?;

-- name: CreateClient :one
INSERT INTO clients (uuid, name, email, phone, address, pricing_tier_id, metadata, payer_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateClient :one
UPDATE clients SET name = ?, email = ?, phone = ?, address = ?, pricing_tier_id = ?, metadata = ?, payer_id = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: DeleteClient :exec
DELETE FROM clients WHERE id = ?;
