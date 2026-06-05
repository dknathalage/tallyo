-- name: ListInvoicePayments :many
SELECT * FROM payments WHERE invoice_id = ? ORDER BY payment_date, id;

-- name: InvoiceTotalPaid :one
SELECT CAST(COALESCE(SUM(amount), 0) AS REAL) AS total_paid FROM payments WHERE invoice_id = ?;

-- name: GetPayment :one
SELECT * FROM payments WHERE id = ?;

-- name: CreatePayment :one
INSERT INTO payments (uuid, invoice_id, amount, payment_date, method, notes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: DeletePayment :exec
DELETE FROM payments WHERE id = ?;
