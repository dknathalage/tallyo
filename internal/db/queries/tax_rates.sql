-- name: ListTaxRates :many
SELECT * FROM tax_rates WHERE tenant_id = $1 ORDER BY is_default DESC, name;

-- name: GetTaxRate :one
SELECT * FROM tax_rates WHERE tenant_id = $1 AND id = $2;

-- name: GetDefaultTaxRate :one
SELECT * FROM tax_rates WHERE tenant_id = $1 AND is_default = 1 LIMIT 1;

-- name: ClearDefaultTaxRates :exec
UPDATE tax_rates SET is_default = 0 WHERE tenant_id = $1;

-- name: CreateTaxRate :one
INSERT INTO tax_rates (id, tenant_id, name, rate, is_default, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateTaxRate :one
UPDATE tax_rates SET name = $1, rate = $2, is_default = $3, updated_at = $4
WHERE tenant_id = $5 AND id = $6 RETURNING *;

-- name: DeleteTaxRate :exec
DELETE FROM tax_rates WHERE tenant_id = $1 AND id = $2;
