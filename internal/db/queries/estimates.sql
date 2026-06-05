-- name: ListEstimates :many
SELECT e.*, c.name AS client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id ORDER BY e.created_at DESC;

-- name: ListEstimatesByStatus :many
SELECT e.*, c.name AS client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id WHERE e.status = ? ORDER BY e.created_at DESC;

-- name: ListClientEstimates :many
SELECT e.*, c.name AS client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id WHERE e.client_id = ? ORDER BY e.created_at DESC;

-- name: GetEstimate :one
SELECT e.*, c.name AS client_name FROM estimates e LEFT JOIN clients c ON e.client_id = c.id WHERE e.id = ?;

-- name: CreateEstimate :one
INSERT INTO estimates (uuid, estimate_number, client_id, date, valid_until, subtotal, tax_rate, tax_rate_id, tax_amount, total, notes, status, currency_code, business_snapshot, client_snapshot, payer_snapshot, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateEstimate :one
UPDATE estimates SET client_id = ?, date = ?, valid_until = ?, subtotal = ?, tax_rate = ?, tax_rate_id = ?, tax_amount = ?, total = ?, notes = ?, status = ?, currency_code = ?, business_snapshot = ?, client_snapshot = ?, payer_snapshot = ?, updated_at = ?
WHERE id = ? RETURNING *;

-- name: UpdateEstimateStatus :exec
UPDATE estimates SET status = ?, updated_at = ? WHERE id = ?;

-- name: SetEstimateConverted :exec
UPDATE estimates SET converted_invoice_id = ?, status = 'converted', updated_at = ? WHERE id = ?;

-- name: DeleteEstimate :exec
DELETE FROM estimates WHERE id = ?;
