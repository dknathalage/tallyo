-- name: ListEstimateLineItems :many
SELECT * FROM estimate_line_items WHERE estimate_id = ? ORDER BY sort_order, id;

-- name: CreateEstimateLineItem :one
INSERT INTO estimate_line_items (uuid, estimate_id, description, quantity, rate, amount, notes, sort_order, catalog_item_id, rate_tier_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: DeleteEstimateLineItemsForEstimate :exec
DELETE FROM estimate_line_items WHERE estimate_id = ?;
