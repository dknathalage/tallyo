-- name: ListEstimateLineItems :many
SELECT eli.*, cat.id AS catalogue_item_uuid
FROM estimate_line_items eli
LEFT JOIN catalogue_items cat ON eli.catalogue_item_id = cat.id
WHERE eli.tenant_id = ? AND eli.estimate_id = ? ORDER BY eli.sort_order, eli.id;

-- name: CreateEstimateLineItem :one
INSERT INTO estimate_line_items (
    id, tenant_id, estimate_id, catalogue_item_id,
    code, description, service_date, unit, quantity, unit_price, taxable, line_total, sort_order
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteEstimateLineItemsForEstimate :exec
DELETE FROM estimate_line_items WHERE tenant_id = ? AND estimate_id = ?;
