-- name: ListEstimateLineItems :many
SELECT eli.*, ci.uuid AS custom_item_uuid
FROM estimate_line_items eli
LEFT JOIN custom_items ci ON eli.custom_item_id = ci.id
WHERE eli.tenant_id = ? AND eli.estimate_id = ? ORDER BY eli.sort_order, eli.id;

-- name: CreateEstimateLineItem :one
INSERT INTO estimate_line_items (
    uuid, tenant_id, estimate_id, item_id, custom_item_id, price_list_version_id,
    code, description, service_date, unit, quantity, unit_price, taxable, line_total, sort_order
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteEstimateLineItemsForEstimate :exec
DELETE FROM estimate_line_items WHERE tenant_id = ? AND estimate_id = ?;
