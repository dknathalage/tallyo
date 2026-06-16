-- name: ListEstimateLineItems :many
SELECT * FROM estimate_line_items WHERE tenant_id = ? AND estimate_id = ? ORDER BY sort_order, id;

-- name: CreateEstimateLineItem :one
INSERT INTO estimate_line_items (
    uuid, tenant_id, estimate_id, support_item_id, custom_item_id, catalog_version_id,
    code, description, service_date, unit, quantity, unit_price, gst_free, line_total, sort_order
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteEstimateLineItemsForEstimate :exec
DELETE FROM estimate_line_items WHERE tenant_id = ? AND estimate_id = ?;
