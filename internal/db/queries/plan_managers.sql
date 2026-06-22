-- name: ListPlanManagers :many
SELECT * FROM plan_managers WHERE tenant_id = ? ORDER BY name;

-- name: SearchPlanManagers :many
SELECT * FROM plan_managers
WHERE tenant_id = ? AND (name LIKE ? OR email LIKE ?)
ORDER BY name;

-- name: GetPlanManager :one
SELECT * FROM plan_managers WHERE tenant_id = ? AND uuid = ?;

-- name: GetPlanManagerIDByUUID :one
SELECT id FROM plan_managers WHERE tenant_id = ? AND uuid = ?;

-- name: GetPlanManagerByID :one
SELECT * FROM plan_managers WHERE tenant_id = ? AND id = ?;

-- name: CreatePlanManager :one
INSERT INTO plan_managers (uuid, tenant_id, name, email, phone, address, metadata, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdatePlanManager :one
UPDATE plan_managers SET name = ?, email = ?, phone = ?, address = ?, metadata = ?, updated_at = ?
WHERE tenant_id = ? AND uuid = ?
RETURNING *;

-- name: DeletePlanManager :exec
DELETE FROM plan_managers WHERE tenant_id = ? AND uuid = ?;

-- name: DeletePlanManagerByID :exec
DELETE FROM plan_managers WHERE tenant_id = ? AND id = ?;
