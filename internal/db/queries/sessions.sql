-- Sessions: the delivered-support unit (tenant-scoped). Physical table work_sessions.
-- Read queries LEFT JOIN clients for p.uuid so the slice DTO can expose the
-- client FK as its uuid (clientId) instead of the internal int, and
-- LEFT JOIN invoices for i.uuid so the linked-invoice FK surfaces as its uuid
-- (invoiceId) instead of the internal int.

-- name: ListSessions :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? ORDER BY s.service_date DESC, s.id DESC;

-- name: ListSessionsByClient :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.client_id = ?
ORDER BY s.service_date, s.id;

-- name: ListSessionsByClientRange :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.client_id = ?
  AND s.service_date >= ? AND s.service_date <= ?
ORDER BY s.service_date, s.id;

-- name: ListSessionsByStatus :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.status = ? ORDER BY s.service_date, s.id;

-- name: ListScheduledSessions :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.status = 'scheduled' ORDER BY s.service_date, s.id;

-- name: ListRecordedUnbilledByClient :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.client_id = ? AND s.status = 'recorded' AND s.invoice_id IS NULL
ORDER BY s.service_date, s.id;

-- name: GetSession :one
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.id = ?;

-- name: GetSessionByID :one
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.id = ?;

-- name: GetSessionIDByUUID :one
SELECT id FROM work_sessions WHERE tenant_id = ? AND id = ?;

-- name: CreateSession :one
INSERT INTO work_sessions (
    id, tenant_id, client_id, service_date, note, tags, status,
    author_user_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateSession :one
UPDATE work_sessions SET
    service_date = ?, note = ?, tags = ?, status = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?
RETURNING *;

-- name: UpdateSessionStatus :exec
UPDATE work_sessions SET status = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: SetSessionInvoice :exec
UPDATE work_sessions SET invoice_id = ?, status = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: SetStatusForInvoice :exec
UPDATE work_sessions SET status = ?, updated_at = ? WHERE tenant_id = ? AND invoice_id = ?;

-- name: ClearSessionsForInvoice :exec
UPDATE work_sessions SET invoice_id = NULL, status = 'recorded', updated_at = ?
WHERE tenant_id = ? AND invoice_id = ?;

-- name: DeleteSession :exec
DELETE FROM work_sessions WHERE tenant_id = ? AND id = ?;

-- name: ClientUnbilledAgg :many
SELECT client_id, COUNT(*) AS cnt, MIN(service_date) AS from_date, MAX(service_date) AS to_date
FROM work_sessions
WHERE tenant_id = ? AND status = 'recorded' AND invoice_id IS NULL
GROUP BY client_id;
