-- name: ListInvoices :many
SELECT i.*, c.name AS client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id ORDER BY i.created_at DESC;

-- name: ListInvoicesByStatus :many
SELECT i.*, c.name AS client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id WHERE i.status = ? ORDER BY i.created_at DESC;

-- name: ListClientInvoices :many
SELECT i.*, c.name AS client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id WHERE i.client_id = ? ORDER BY i.created_at DESC;

-- name: GetInvoice :one
SELECT i.*, c.name AS client_name FROM invoices i LEFT JOIN clients c ON i.client_id = c.id WHERE i.id = ?;

-- name: CreateInvoice :one
INSERT INTO invoices (uuid, invoice_number, client_id, date, due_date, payment_terms, subtotal, tax_rate, tax_rate_id, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateInvoice :one
UPDATE invoices SET client_id = ?, date = ?, due_date = ?, payment_terms = ?, subtotal = ?, tax_rate = ?, tax_rate_id = ?, tax_amount = ?, total = ?, notes = ?, status = ?, currency_code = ?, business_snapshot = ?, client_snapshot = ?, payer_snapshot = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: UpdateInvoiceStatus :exec
UPDATE invoices SET status = ?, updated_at = ? WHERE id = ?;

-- name: DeleteInvoice :exec
DELETE FROM invoices WHERE id = ?;

-- name: SelectOverdueInvoices :many
SELECT id, invoice_number FROM invoices WHERE status = 'sent' AND due_date < date('now');

-- name: ClientInvoiceStats :one
SELECT COUNT(*) AS invoice_count, CAST(COALESCE(SUM(total), 0) AS REAL) AS total_invoiced FROM invoices WHERE client_id = ?;
