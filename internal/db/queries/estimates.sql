-- name: ListEstimates :many
SELECT e.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid, ci.id AS converted_invoice_uuid
FROM estimates e
LEFT JOIN clients p ON e.client_id = p.id AND p.tenant_id = e.tenant_id
LEFT JOIN payers pm ON e.payer_id = pm.id AND pm.tenant_id = e.tenant_id
LEFT JOIN invoices ci ON e.converted_invoice_id = ci.id AND ci.tenant_id = e.tenant_id
WHERE e.tenant_id = ?
ORDER BY e.created_at DESC;

-- name: ListEstimatesByStatus :many
SELECT e.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid, ci.id AS converted_invoice_uuid
FROM estimates e
LEFT JOIN clients p ON e.client_id = p.id AND p.tenant_id = e.tenant_id
LEFT JOIN payers pm ON e.payer_id = pm.id AND pm.tenant_id = e.tenant_id
LEFT JOIN invoices ci ON e.converted_invoice_id = ci.id AND ci.tenant_id = e.tenant_id
WHERE e.tenant_id = ? AND e.status = ?
ORDER BY e.created_at DESC;

-- name: ListClientEstimates :many
SELECT e.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid, ci.id AS converted_invoice_uuid
FROM estimates e
LEFT JOIN clients p ON e.client_id = p.id AND p.tenant_id = e.tenant_id
LEFT JOIN payers pm ON e.payer_id = pm.id AND pm.tenant_id = e.tenant_id
LEFT JOIN invoices ci ON e.converted_invoice_id = ci.id AND ci.tenant_id = e.tenant_id
WHERE e.tenant_id = ? AND e.client_id = ?
ORDER BY e.created_at DESC;

-- name: GetEstimate :one
SELECT e.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid, ci.id AS converted_invoice_uuid
FROM estimates e
LEFT JOIN clients p ON e.client_id = p.id AND p.tenant_id = e.tenant_id
LEFT JOIN payers pm ON e.payer_id = pm.id AND pm.tenant_id = e.tenant_id
LEFT JOIN invoices ci ON e.converted_invoice_id = ci.id AND ci.tenant_id = e.tenant_id
WHERE e.tenant_id = ? AND e.id = ?;

-- name: GetEstimateByID :one
SELECT e.*, p.name AS client_name, p.id AS client_uuid, pm.id AS payer_uuid, ci.id AS converted_invoice_uuid
FROM estimates e
LEFT JOIN clients p ON e.client_id = p.id AND p.tenant_id = e.tenant_id
LEFT JOIN payers pm ON e.payer_id = pm.id AND pm.tenant_id = e.tenant_id
LEFT JOIN invoices ci ON e.converted_invoice_id = ci.id AND ci.tenant_id = e.tenant_id
WHERE e.tenant_id = ? AND e.id = ?;

-- name: GetEstimateIDByUUID :one
SELECT id FROM estimates WHERE tenant_id = ? AND id = ?;

-- name: CreateEstimate :one
INSERT INTO estimates (
    id, tenant_id, number, client_id, payer_id, status, issue_date, valid_until,
    subtotal, tax, total, notes, converted_invoice_id,
    business_snapshot, client_snapshot, payer_snapshot, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateEstimate :one
UPDATE estimates SET
    number = ?, client_id = ?, payer_id = ?, status = ?, issue_date = ?, valid_until = ?,
    subtotal = ?, tax = ?, total = ?, notes = ?,
    business_snapshot = ?, client_snapshot = ?, payer_snapshot = ?, updated_at = ?
WHERE tenant_id = ? AND id = ?
RETURNING *;

-- name: UpdateEstimateStatus :exec
UPDATE estimates SET status = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: SetEstimateConverted :exec
UPDATE estimates SET converted_invoice_id = ?, status = 'converted', updated_at = ?
WHERE tenant_id = ? AND id = ?;

-- name: MaxEstimateNumberLike :one
-- Highest numeric sequence (parsed from the suffix after prefix_len chars),
-- pad-width independent. prefix_len is the length of the non-numeric prefix
-- (e.g. 4 for 'EST-'); the numeric part begins at prefix_len + 1.
SELECT CAST(COALESCE(MAX(CAST(substr(number, CAST(sqlc.arg(prefix_len) AS INTEGER) + 1) AS INTEGER)), 0) AS INTEGER) AS max_seq
FROM estimates
WHERE tenant_id = sqlc.arg(tenant_id) AND number LIKE sqlc.arg(pattern);

-- name: DeleteEstimate :exec
DELETE FROM estimates WHERE tenant_id = ? AND id = ?;
