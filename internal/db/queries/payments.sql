-- name: ListInvoicePayments :many
SELECT * FROM payments WHERE tenant_id = $1 AND invoice_id = $2 ORDER BY paid_at, id;

-- name: InvoiceTotalPaid :one
SELECT CAST(COALESCE(SUM(amount), 0) AS double precision) AS total_paid
FROM payments WHERE tenant_id = $1 AND invoice_id = $2;

-- name: GetPayment :one
SELECT * FROM payments WHERE tenant_id = $1 AND id = $2;

-- name: CreatePayment :one
INSERT INTO payments (id, tenant_id, invoice_id, amount, paid_at, method, reference, notes, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: DeletePayment :exec
DELETE FROM payments WHERE tenant_id = $1 AND id = $2;

-- name: GetPaymentByUUID :one
SELECT * FROM payments WHERE tenant_id = $1 AND invoice_id = $2 AND id = $3;

-- name: DeletePaymentByUUID :exec
DELETE FROM payments WHERE tenant_id = $1 AND invoice_id = $2 AND id = $3;
