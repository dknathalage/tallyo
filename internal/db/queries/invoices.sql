-- name: ListInvoices :many
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = ?
ORDER BY i.created_at DESC;

-- name: ListInvoicesByStatus :many
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = ? AND i.status = ?
ORDER BY i.created_at DESC;

-- name: ListClientInvoices :many
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = ? AND i.client_id = ?
ORDER BY i.created_at DESC;

-- name: GetInvoice :one
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = ? AND i.id = ?;

-- name: GetInvoiceByID :one
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = ? AND i.id = ?;

-- name: GetInvoiceIDByUUID :one
SELECT id FROM invoices WHERE tenant_id = ? AND id = ?;

-- name: CreateInvoice :one
INSERT INTO invoices (
    id, tenant_id, number, client_id, payer_id, status, issue_date, due_date,
    subtotal, tax, total, notes, business_snapshot, client_snapshot, payer_snapshot,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateInvoice :one
UPDATE invoices SET
    number = ?, client_id = ?, payer_id = ?, status = ?, issue_date = ?, due_date = ?,
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

-- name: ClientInvoiceStats :one
SELECT
  COUNT(*) AS invoice_count,
  CAST(COALESCE(SUM(i.total), 0) AS REAL) AS total_invoiced,
  CAST(COALESCE((
    SELECT SUM(p.amount) FROM payments p
    JOIN invoices iv ON p.invoice_id = iv.id
    WHERE iv.tenant_id = sqlc.arg(tenant_id) AND iv.client_id = sqlc.arg(client_id)
  ), 0) AS REAL) AS total_paid
FROM invoices i
WHERE i.tenant_id = sqlc.arg(tenant_id) AND i.client_id = sqlc.arg(client_id);
