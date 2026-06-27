-- name: ListLineItemsForInvoice :many
SELECT li.*, cat.id AS catalogue_item_uuid
FROM line_items li
LEFT JOIN catalogue_items cat ON li.catalogue_item_id = cat.id
WHERE li.tenant_id = $1 AND li.invoice_id = $2 ORDER BY li.sort_order, li.id;

-- name: ListLineItemsForSession :many
SELECT li.*, cat.id AS catalogue_item_uuid
FROM line_items li
LEFT JOIN catalogue_items cat ON li.catalogue_item_id = cat.id
WHERE li.tenant_id = $1 AND li.session_id = $2 ORDER BY li.id;

-- name: GetLineItem :one
SELECT li.*, cat.id AS catalogue_item_uuid
FROM line_items li
LEFT JOIN catalogue_items cat ON li.catalogue_item_id = cat.id
WHERE li.tenant_id = $1 AND li.id = $2;

-- name: GetSessionLineItemByUUID :one
-- A session's line item addressed by its uuid, scoped to the owning session's int id.
SELECT li.*, cat.id AS catalogue_item_uuid
FROM line_items li
LEFT JOIN catalogue_items cat ON li.catalogue_item_id = cat.id
WHERE li.tenant_id = $1 AND li.session_id = $2 AND li.id = $3;

-- name: CreateLineItem :one
INSERT INTO line_items (
    id, tenant_id, session_id, invoice_id, catalogue_item_id, code, description,
    service_date, unit, start_time, end_time, quantity, unit_price, taxable,
    line_total, sort_order
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: UpdateSessionLineItem :one
UPDATE line_items SET
    catalogue_item_id = $1, code = $2, description = $3, service_date = $4, unit = $5,
    start_time = $6, end_time = $7, quantity = $8, unit_price = $9, taxable = $10, line_total = $11
WHERE tenant_id = $12 AND id = $13 AND invoice_id IS NULL
RETURNING *;

-- name: DeleteSessionLineItem :exec
DELETE FROM line_items WHERE tenant_id = $1 AND id = $2 AND invoice_id IS NULL;

-- name: UpdateSessionLineItemByUUID :one
-- Rewrite one UNBILLED session item addressed by uuid, scoped to the owning session.
UPDATE line_items SET
    catalogue_item_id = $1, code = $2, description = $3, service_date = $4, unit = $5,
    start_time = $6, end_time = $7, quantity = $8, unit_price = $9, taxable = $10, line_total = $11
WHERE tenant_id = $12 AND session_id = $13 AND id = $14 AND invoice_id IS NULL
RETURNING *;

-- name: DeleteSessionLineItemByUUID :exec
DELETE FROM line_items WHERE tenant_id = $1 AND session_id = $2 AND id = $3 AND invoice_id IS NULL;

-- name: DeleteUnbilledItemsForSession :exec
DELETE FROM line_items WHERE tenant_id = $1 AND session_id = $2 AND invoice_id IS NULL;

-- name: CountSessionItems :one
SELECT COUNT(*) FROM line_items WHERE tenant_id = $1 AND session_id = $2 AND invoice_id IS NULL;

-- name: LinkSessionItemsToInvoice :exec
UPDATE line_items SET invoice_id = $1, sort_order = $2
WHERE tenant_id = $3 AND session_id = $4 AND invoice_id IS NULL;

-- name: RestampUnbilledSessionItems :exec
UPDATE line_items SET service_date = $1
WHERE tenant_id = $2 AND session_id = $3 AND invoice_id IS NULL;

-- name: UnlinkSessionItemsFromInvoice :exec
UPDATE line_items SET invoice_id = NULL, sort_order = 0
WHERE tenant_id = $1 AND invoice_id = $2 AND session_id IS NOT NULL;

-- name: DeleteLineItemsForInvoice :exec
DELETE FROM line_items WHERE tenant_id = $1 AND invoice_id = $2;
