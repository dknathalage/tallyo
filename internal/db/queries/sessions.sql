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
WHERE s.tenant_id = $1 ORDER BY s.service_date DESC, s.id DESC;

-- name: ListSessionsByClient :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = $1 AND s.client_id = $2
ORDER BY s.service_date, s.id;

-- name: ListSessionsByClientRange :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = $1 AND s.client_id = $2
  AND s.service_date >= $3 AND s.service_date <= $4
ORDER BY s.service_date, s.id;

-- name: ListSessionsByStatus :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = $1 AND s.status = $2 ORDER BY s.service_date, s.id;

-- name: ListScheduledSessions :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = $1 AND s.status = 'scheduled' ORDER BY s.service_date, s.id;

-- name: ListRecordedUnbilledByClient :many
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = $1 AND s.client_id = $2 AND s.status = 'recorded' AND s.invoice_id IS NULL
ORDER BY s.service_date, s.id;

-- name: GetSession :one
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = $1 AND s.id = $2;

-- name: GetSessionByID :one
SELECT s.*, p.id AS client_uuid, i.id AS invoice_uuid
FROM work_sessions s
LEFT JOIN clients p ON s.client_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = $1 AND s.id = $2;

-- name: GetSessionIDByUUID :one
SELECT id FROM work_sessions WHERE tenant_id = $1 AND id = $2;

-- name: CreateSession :one
INSERT INTO work_sessions (
    id, tenant_id, client_id, service_date, note, tags, status,
    author_user_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpdateSession :one
UPDATE work_sessions SET
    service_date = $1, note = $2, tags = $3, status = $4, updated_at = $5
WHERE tenant_id = $6 AND id = $7
RETURNING *;

-- name: UpdateSessionStatus :exec
UPDATE work_sessions SET status = $1, updated_at = $2 WHERE tenant_id = $3 AND id = $4;

-- name: SetSessionInvoice :exec
UPDATE work_sessions SET invoice_id = $1, status = $2, updated_at = $3 WHERE tenant_id = $4 AND id = $5;

-- name: SetStatusForInvoice :exec
UPDATE work_sessions SET status = $1, updated_at = $2 WHERE tenant_id = $3 AND invoice_id = $4;

-- name: ClearSessionsForInvoice :exec
UPDATE work_sessions SET invoice_id = NULL, status = 'recorded', updated_at = $1
WHERE tenant_id = $2 AND invoice_id = $3;

-- name: DeleteSession :exec
DELETE FROM work_sessions WHERE tenant_id = $1 AND id = $2;

-- name: ClientUnbilledAgg :many
SELECT client_id, COUNT(*) AS cnt, MIN(service_date) AS from_date, MAX(service_date) AS to_date
FROM work_sessions
WHERE tenant_id = $1 AND status = 'recorded' AND invoice_id IS NULL
GROUP BY client_id;
