-- name: ListRecurringTemplates :many
SELECT r.*, c.name AS client_name FROM recurring_templates r LEFT JOIN clients c ON r.client_id = c.id ORDER BY r.next_due;

-- name: ListActiveRecurringTemplates :many
SELECT r.*, c.name AS client_name FROM recurring_templates r LEFT JOIN clients c ON r.client_id = c.id WHERE r.is_active = 1 ORDER BY r.next_due;

-- name: GetRecurringTemplate :one
SELECT r.*, c.name AS client_name FROM recurring_templates r LEFT JOIN clients c ON r.client_id = c.id WHERE r.id = ?;

-- name: ListDueTemplates :many
SELECT r.*, c.name AS client_name FROM recurring_templates r LEFT JOIN clients c ON r.client_id = c.id WHERE r.is_active = 1 AND r.next_due <= ? ORDER BY r.next_due;

-- name: CreateRecurringTemplate :one
INSERT INTO recurring_templates (uuid, client_id, name, frequency, next_due, line_items, tax_rate, notes, is_active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateRecurringTemplate :one
UPDATE recurring_templates SET client_id = ?, name = ?, frequency = ?, next_due = ?, line_items = ?, tax_rate = ?, notes = ?, is_active = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: SetRecurringNextDue :exec
UPDATE recurring_templates SET next_due = ?, updated_at = ? WHERE id = ?;

-- name: DeleteRecurringTemplate :exec
DELETE FROM recurring_templates WHERE id = ?;
