-- name: ListInvoices :many
SELECT i.*, p.name AS participant_name
FROM invoices i
LEFT JOIN participants p ON i.participant_id = p.id AND p.tenant_id = i.tenant_id
WHERE i.tenant_id = ?
ORDER BY i.created_at DESC;

-- name: ListInvoicesByStatus :many
SELECT i.*, p.name AS participant_name
FROM invoices i
LEFT JOIN participants p ON i.participant_id = p.id AND p.tenant_id = i.tenant_id
WHERE i.tenant_id = ? AND i.status = ?
ORDER BY i.created_at DESC;

-- name: ListParticipantInvoices :many
SELECT i.*, p.name AS participant_name
FROM invoices i
LEFT JOIN participants p ON i.participant_id = p.id AND p.tenant_id = i.tenant_id
WHERE i.tenant_id = ? AND i.participant_id = ?
ORDER BY i.created_at DESC;

-- name: GetInvoice :one
SELECT i.*, p.name AS participant_name
FROM invoices i
LEFT JOIN participants p ON i.participant_id = p.id AND p.tenant_id = i.tenant_id
WHERE i.tenant_id = ? AND i.id = ?;

-- name: CreateInvoice :one
INSERT INTO invoices (
    uuid, tenant_id, number, participant_id, plan_manager_id, status, issue_date, due_date,
    subtotal, tax, total, notes, business_snapshot, client_snapshot, payer_snapshot,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateInvoice :one
UPDATE invoices SET
    number = ?, participant_id = ?, plan_manager_id = ?, status = ?, issue_date = ?, due_date = ?,
    subtotal = ?, tax = ?, total = ?, notes = ?,
    business_snapshot = ?, client_snapshot = ?, payer_snapshot = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?
RETURNING *;

-- name: UpdateInvoiceStatus :exec
UPDATE invoices SET status = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: UpdateInvoiceTotals :one
UPDATE invoices SET subtotal = ?, tax = ?, total = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?
RETURNING *;

-- name: DeleteInvoice :exec
DELETE FROM invoices WHERE tenant_id = ? AND id = ?;

-- name: MaxInvoiceNumberLike :one
-- Highest numeric sequence (parsed from the suffix after prefix_len chars),
-- pad-width independent. prefix_len is the length of the non-numeric prefix
-- (e.g. 4 for 'INV-'); the numeric part begins at prefix_len + 1.
SELECT CAST(COALESCE(MAX(CAST(substr(number, CAST(sqlc.arg(prefix_len) AS INTEGER) + 1) AS INTEGER)), 0) AS INTEGER) AS max_seq
FROM invoices
WHERE tenant_id = sqlc.arg(tenant_id) AND number LIKE sqlc.arg(pattern);

-- name: SelectOverdueInvoicesForTenant :many
SELECT id, tenant_id, number FROM invoices
WHERE tenant_id = ? AND status = 'sent' AND due_date < date('now');

-- name: ParticipantInvoiceStats :one
SELECT
  COUNT(*) AS invoice_count,
  CAST(COALESCE(SUM(i.total), 0) AS REAL) AS total_invoiced,
  CAST(COALESCE((
    SELECT SUM(p.amount) FROM payments p
    JOIN invoices iv ON p.invoice_id = iv.id
    WHERE iv.tenant_id = sqlc.arg(tenant_id) AND iv.participant_id = sqlc.arg(participant_id)
  ), 0) AS REAL) AS total_paid
FROM invoices i
WHERE i.tenant_id = sqlc.arg(tenant_id) AND i.participant_id = sqlc.arg(participant_id);
