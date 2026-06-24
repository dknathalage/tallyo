-- name: ListInvoicePayments :many
SELECT * FROM payments WHERE tenant_id = ? AND invoice_id = ? ORDER BY paid_at, id;

-- name: InvoiceTotalPaid :one
SELECT CAST(COALESCE(SUM(amount), 0) AS REAL) AS total_paid
FROM payments WHERE tenant_id = ? AND invoice_id = ?;

-- name: GetPayment :one
SELECT * FROM payments WHERE tenant_id = ? AND id = ?;

-- name: CreatePayment :one
INSERT INTO payments (id, tenant_id, invoice_id, amount, paid_at, method, reference, notes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: DeletePayment :exec
DELETE FROM payments WHERE tenant_id = ? AND id = ?;

-- name: GetPaymentByUUID :one
SELECT * FROM payments WHERE tenant_id = ? AND invoice_id = ? AND id = ?;

-- name: DeletePaymentByUUID :exec
DELETE FROM payments WHERE tenant_id = ? AND invoice_id = ? AND id = ?;
