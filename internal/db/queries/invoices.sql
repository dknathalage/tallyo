-- name: ListInvoices :many
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = $1
ORDER BY i.created_at DESC;

-- name: ListInvoicesByStatus :many
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = $1 AND i.status = $2
ORDER BY i.created_at DESC;

-- name: ListClientInvoices :many
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = $1 AND i.client_id = $2
ORDER BY i.created_at DESC;

-- name: GetInvoice :one
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = $1 AND i.id = $2;

-- name: GetInvoiceByID :one
SELECT i.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid
FROM invoices i
LEFT JOIN clients p ON i.client_id = p.id AND p.tenant_id = i.tenant_id
LEFT JOIN payers pm ON i.payer_id = pm.id AND pm.tenant_id = i.tenant_id
WHERE i.tenant_id = $1 AND i.id = $2;

-- name: GetInvoiceIDByUUID :one
SELECT id FROM invoices WHERE tenant_id = $1 AND id = $2;

-- name: CreateInvoice :one
INSERT INTO invoices (
    id, tenant_id, number, client_id, payer_id, status, issue_date, due_date,
    subtotal, tax, total, notes, business_snapshot, client_snapshot, payer_snapshot,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING *;

-- name: UpdateInvoice :one
UPDATE invoices SET
    number = $1, client_id = $2, payer_id = $3, status = $4, issue_date = $5, due_date = $6,
    subtotal = $7, tax = $8, total = $9, notes = $10,
    business_snapshot = $11, client_snapshot = $12, payer_snapshot = $13, updated_at = $14
WHERE tenant_id = $15 AND id = $16
RETURNING *;

-- name: UpdateInvoiceStatus :exec
UPDATE invoices SET status = $1, updated_at = $2 WHERE tenant_id = $3 AND id = $4;

-- name: UpdateInvoiceTotals :one
UPDATE invoices SET subtotal = $1, tax = $2, total = $3, updated_at = $4
WHERE tenant_id = $5 AND id = $6
RETURNING *;

-- name: DeleteInvoice :exec
DELETE FROM invoices WHERE tenant_id = $1 AND id = $2;

-- name: MaxInvoiceNumberLike :one
-- Highest numeric sequence (parsed from the suffix after prefix_len chars),
-- pad-width independent. prefix_len is the length of the non-numeric prefix
-- (e.g. 4 for 'INV-'); the numeric part begins at prefix_len + 1.
SELECT CAST(COALESCE(MAX(CAST(substr(number, CAST(sqlc.arg(prefix_len) AS INTEGER) + 1) AS INTEGER)), 0) AS INTEGER) AS max_seq
FROM invoices
WHERE tenant_id = sqlc.arg(tenant_id) AND number LIKE sqlc.arg(pattern);

-- name: ClientInvoiceStats :one
SELECT
  COUNT(*) AS invoice_count,
  CAST(COALESCE(SUM(i.total), 0) AS double precision) AS total_invoiced,
  CAST(COALESCE((
    SELECT SUM(p.amount) FROM payments p
    JOIN invoices iv ON p.invoice_id = iv.id
    WHERE iv.tenant_id = sqlc.arg(tenant_id) AND iv.client_id = sqlc.arg(client_id)
  ), 0) AS double precision) AS total_paid
FROM invoices i
WHERE i.tenant_id = sqlc.arg(tenant_id) AND i.client_id = sqlc.arg(client_id);
