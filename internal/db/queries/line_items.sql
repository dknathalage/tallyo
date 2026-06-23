-- name: ListLineItemsForInvoice :many
SELECT li.*, ci.uuid AS custom_item_uuid
FROM line_items li
LEFT JOIN custom_items ci ON li.custom_item_id = ci.id
WHERE li.tenant_id = ? AND li.invoice_id = ? ORDER BY li.sort_order, li.id;

-- name: ListLineItemsForSession :many
SELECT li.*, ci.uuid AS custom_item_uuid
FROM line_items li
LEFT JOIN custom_items ci ON li.custom_item_id = ci.id
WHERE li.tenant_id = ? AND li.session_id = ? ORDER BY li.id;

-- name: GetLineItem :one
SELECT li.*, ci.uuid AS custom_item_uuid
FROM line_items li
LEFT JOIN custom_items ci ON li.custom_item_id = ci.id
WHERE li.tenant_id = ? AND li.id = ?;

-- name: GetSessionLineItemByUUID :one
-- A session's line item addressed by its uuid, scoped to the owning session's int id.
SELECT li.*, ci.uuid AS custom_item_uuid
FROM line_items li
LEFT JOIN custom_items ci ON li.custom_item_id = ci.id
WHERE li.tenant_id = ? AND li.session_id = ? AND li.uuid = ?;

-- name: CreateLineItem :one
INSERT INTO line_items (
    uuid, tenant_id, session_id, invoice_id, support_item_id, custom_item_id,
    catalog_version_id, code, description, service_date, unit, start_time,
    end_time, quantity, unit_price, taxable, line_total, sort_order
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateSessionLineItem :one
UPDATE line_items SET
    support_item_id = ?, custom_item_id = ?, catalog_version_id = ?, code = ?,
    description = ?, service_date = ?, unit = ?, start_time = ?, end_time = ?,
    quantity = ?, unit_price = ?, taxable = ?, line_total = ?
WHERE tenant_id = ? AND id = ? AND invoice_id IS NULL
RETURNING *;

-- name: DeleteSessionLineItem :exec
DELETE FROM line_items WHERE tenant_id = ? AND id = ? AND invoice_id IS NULL;

-- name: UpdateSessionLineItemByUUID :one
-- Rewrite one UNBILLED session item addressed by uuid, scoped to the owning session.
UPDATE line_items SET
    support_item_id = ?, custom_item_id = ?, catalog_version_id = ?, code = ?,
    description = ?, service_date = ?, unit = ?, start_time = ?, end_time = ?,
    quantity = ?, unit_price = ?, taxable = ?, line_total = ?
WHERE tenant_id = ? AND session_id = ? AND uuid = ? AND invoice_id IS NULL
RETURNING *;

-- name: DeleteSessionLineItemByUUID :exec
DELETE FROM line_items WHERE tenant_id = ? AND session_id = ? AND uuid = ? AND invoice_id IS NULL;

-- name: DeleteUnbilledItemsForSession :exec
DELETE FROM line_items WHERE tenant_id = ? AND session_id = ? AND invoice_id IS NULL;

-- name: CountSessionItems :one
SELECT COUNT(*) FROM line_items WHERE tenant_id = ? AND session_id = ? AND invoice_id IS NULL;

-- name: LinkSessionItemsToInvoice :exec
UPDATE line_items SET invoice_id = ?, sort_order = ?
WHERE tenant_id = ? AND session_id = ? AND invoice_id IS NULL;

-- name: RestampUnbilledSessionItems :exec
UPDATE line_items SET service_date = ?
WHERE tenant_id = ? AND session_id = ? AND invoice_id IS NULL;

-- name: UnlinkSessionItemsFromInvoice :exec
UPDATE line_items SET invoice_id = NULL, sort_order = 0
WHERE tenant_id = ? AND invoice_id = ? AND session_id IS NOT NULL;

-- name: DeleteLineItemsForInvoice :exec
DELETE FROM line_items WHERE tenant_id = ? AND invoice_id = ?;
