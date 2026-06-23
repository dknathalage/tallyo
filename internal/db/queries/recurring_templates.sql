-- name: ListRecurringTemplates :many
SELECT r.*, p.name AS client_name, p.uuid AS client_uuid, pm.uuid AS plan_manager_uuid
FROM recurring_templates r
LEFT JOIN clients p ON r.client_id = p.id AND p.tenant_id = r.tenant_id
LEFT JOIN plan_managers pm ON r.plan_manager_id = pm.id AND pm.tenant_id = r.tenant_id
WHERE r.tenant_id = ?
ORDER BY r.next_due;

-- name: ListActiveRecurringTemplates :many
SELECT r.*, p.name AS client_name, p.uuid AS client_uuid, pm.uuid AS plan_manager_uuid
FROM recurring_templates r
LEFT JOIN clients p ON r.client_id = p.id AND p.tenant_id = r.tenant_id
LEFT JOIN plan_managers pm ON r.plan_manager_id = pm.id AND pm.tenant_id = r.tenant_id
WHERE r.tenant_id = ? AND r.is_active = 1
ORDER BY r.next_due;

-- name: GetRecurringTemplate :one
SELECT r.*, p.name AS client_name, p.uuid AS client_uuid, pm.uuid AS plan_manager_uuid
FROM recurring_templates r
LEFT JOIN clients p ON r.client_id = p.id AND p.tenant_id = r.tenant_id
LEFT JOIN plan_managers pm ON r.plan_manager_id = pm.id AND pm.tenant_id = r.tenant_id
WHERE r.tenant_id = ? AND r.uuid = ?;

-- name: ListDueTemplatesForTenant :many
SELECT * FROM recurring_templates
WHERE tenant_id = ? AND is_active = 1 AND next_due <= ?
ORDER BY next_due;

-- name: CreateRecurringTemplate :one
INSERT INTO recurring_templates (
    uuid, tenant_id, client_id, plan_manager_id, name, frequency, next_due,
    line_items, tax_rate, notes, is_active, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateRecurringTemplate :one
UPDATE recurring_templates SET
    client_id = ?, plan_manager_id = ?, name = ?, frequency = ?, next_due = ?,
    line_items = ?, tax_rate = ?, notes = ?, is_active = ?, updated_at = ?
WHERE tenant_id = ? AND uuid = ?
RETURNING *;

-- name: SetRecurringNextDue :exec
UPDATE recurring_templates SET next_due = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: DeleteRecurringTemplate :exec
DELETE FROM recurring_templates WHERE tenant_id = ? AND uuid = ?;
