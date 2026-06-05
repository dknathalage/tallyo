-- name: ListTaxRates :many
SELECT * FROM tax_rates ORDER BY is_default DESC, name;

-- name: GetTaxRate :one
SELECT * FROM tax_rates WHERE id = ?;

-- name: GetDefaultTaxRate :one
SELECT * FROM tax_rates WHERE is_default = 1 LIMIT 1;

-- name: ClearDefaultTaxRates :exec
UPDATE tax_rates SET is_default = 0;

-- name: CreateTaxRate :one
INSERT INTO tax_rates (uuid, name, rate, is_default, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateTaxRate :one
UPDATE tax_rates SET name = ?, rate = ?, is_default = ?, updated_at = ? WHERE id = ? RETURNING *;

-- name: DeleteTaxRate :exec
DELETE FROM tax_rates WHERE id = ?;
