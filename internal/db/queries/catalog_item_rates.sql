-- name: UpsertCatalogItemRate :exec
INSERT INTO catalog_item_rates (catalog_item_id, rate_tier_id, rate)
VALUES (?, ?, ?)
ON CONFLICT(catalog_item_id, rate_tier_id) DO UPDATE SET rate = excluded.rate;

-- name: GetCatalogItemRate :one
SELECT * FROM catalog_item_rates WHERE catalog_item_id = ? AND rate_tier_id = ?;

-- name: ListRatesForItem :many
SELECT * FROM catalog_item_rates WHERE catalog_item_id = ?;

-- name: DeleteCatalogItemRate :exec
DELETE FROM catalog_item_rates WHERE catalog_item_id = ? AND rate_tier_id = ?;
