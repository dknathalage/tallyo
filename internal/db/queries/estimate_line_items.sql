-- name: ListEstimateLineItems :many
SELECT eli.*, cat.id AS catalogue_item_uuid
FROM estimate_line_items eli
LEFT JOIN catalogue_items cat ON eli.catalogue_item_id = cat.id
WHERE eli.tenant_id = $1 AND eli.estimate_id = $2 ORDER BY eli.sort_order, eli.id;

-- name: CreateEstimateLineItem :one
INSERT INTO estimate_line_items (
    id, tenant_id, estimate_id, catalogue_item_id,
    code, description, service_date, unit, quantity, unit_price, taxable, line_total, sort_order
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: DeleteEstimateLineItemsForEstimate :exec
DELETE FROM estimate_line_items WHERE tenant_id = $1 AND estimate_id = $2;
