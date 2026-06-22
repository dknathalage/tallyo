-- name: ListParticipants :many
SELECT p.*, pm.name AS plan_manager_name, pm.uuid AS plan_manager_uuid
FROM participants p
LEFT JOIN plan_managers pm ON p.plan_manager_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ?
ORDER BY p.name;

-- name: SearchParticipants :many
SELECT p.*, pm.name AS plan_manager_name, pm.uuid AS plan_manager_uuid
FROM participants p
LEFT JOIN plan_managers pm ON p.plan_manager_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ? AND (p.name LIKE ? OR p.email LIKE ? OR p.ndis_number LIKE ?)
ORDER BY p.name;

-- name: GetParticipant :one
SELECT p.*, pm.name AS plan_manager_name, pm.uuid AS plan_manager_uuid
FROM participants p
LEFT JOIN plan_managers pm ON p.plan_manager_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ? AND p.uuid = ?;

-- name: GetParticipantIDByUUID :one
SELECT id FROM participants WHERE tenant_id = ? AND uuid = ?;

-- name: GetParticipantByID :one
SELECT p.*, pm.name AS plan_manager_name, pm.uuid AS plan_manager_uuid
FROM participants p
LEFT JOIN plan_managers pm ON p.plan_manager_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ? AND p.id = ?;

-- name: CreateParticipant :one
INSERT INTO participants (
    uuid, tenant_id, name, ndis_number, plan_start, plan_end, mgmt_type, plan_manager_id,
    email, phone, address, metadata, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateParticipant :one
UPDATE participants SET
    name = ?, ndis_number = ?, plan_start = ?, plan_end = ?, mgmt_type = ?, plan_manager_id = ?,
    email = ?, phone = ?, address = ?, metadata = ?, updated_at = ?
WHERE tenant_id = ? AND uuid = ?
RETURNING *;

-- name: DeleteParticipant :exec
DELETE FROM participants WHERE tenant_id = ? AND uuid = ?;

-- name: DeleteParticipantByID :exec
DELETE FROM participants WHERE tenant_id = ? AND id = ?;
