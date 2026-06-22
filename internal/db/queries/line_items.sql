-- name: ListLineItemsForInvoice :many
SELECT li.*, ci.uuid AS custom_item_uuid
FROM line_items li
LEFT JOIN custom_items ci ON li.custom_item_id = ci.id
WHERE li.tenant_id = ? AND li.invoice_id = ? ORDER BY li.sort_order, li.id;

-- name: ListLineItemsForShift :many
SELECT li.*, ci.uuid AS custom_item_uuid
FROM line_items li
LEFT JOIN custom_items ci ON li.custom_item_id = ci.id
WHERE li.tenant_id = ? AND li.shift_id = ? ORDER BY li.id;

-- name: GetLineItem :one
SELECT li.*, ci.uuid AS custom_item_uuid
FROM line_items li
LEFT JOIN custom_items ci ON li.custom_item_id = ci.id
WHERE li.tenant_id = ? AND li.id = ?;

-- name: GetShiftLineItemByUUID :one
-- A shift's line item addressed by its uuid, scoped to the owning shift's int id.
SELECT li.*, ci.uuid AS custom_item_uuid
FROM line_items li
LEFT JOIN custom_items ci ON li.custom_item_id = ci.id
WHERE li.tenant_id = ? AND li.shift_id = ? AND li.uuid = ?;

-- name: CreateLineItem :one
INSERT INTO line_items (
    uuid, tenant_id, shift_id, invoice_id, support_item_id, custom_item_id,
    catalog_version_id, code, description, service_date, unit, start_time,
    end_time, quantity, unit_price, taxable, line_total, sort_order
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateShiftLineItem :one
UPDATE line_items SET
    support_item_id = ?, custom_item_id = ?, catalog_version_id = ?, code = ?,
    description = ?, service_date = ?, unit = ?, start_time = ?, end_time = ?,
    quantity = ?, unit_price = ?, taxable = ?, line_total = ?
WHERE tenant_id = ? AND id = ? AND invoice_id IS NULL
RETURNING *;

-- name: DeleteShiftLineItem :exec
DELETE FROM line_items WHERE tenant_id = ? AND id = ? AND invoice_id IS NULL;

-- name: UpdateShiftLineItemByUUID :one
-- Rewrite one UNBILLED shift item addressed by uuid, scoped to the owning shift.
UPDATE line_items SET
    support_item_id = ?, custom_item_id = ?, catalog_version_id = ?, code = ?,
    description = ?, service_date = ?, unit = ?, start_time = ?, end_time = ?,
    quantity = ?, unit_price = ?, taxable = ?, line_total = ?
WHERE tenant_id = ? AND shift_id = ? AND uuid = ? AND invoice_id IS NULL
RETURNING *;

-- name: DeleteShiftLineItemByUUID :exec
DELETE FROM line_items WHERE tenant_id = ? AND shift_id = ? AND uuid = ? AND invoice_id IS NULL;

-- name: DeleteUnbilledItemsForShift :exec
DELETE FROM line_items WHERE tenant_id = ? AND shift_id = ? AND invoice_id IS NULL;

-- name: CountShiftItems :one
SELECT COUNT(*) FROM line_items WHERE tenant_id = ? AND shift_id = ? AND invoice_id IS NULL;

-- name: LinkShiftItemsToInvoice :exec
UPDATE line_items SET invoice_id = ?, sort_order = ?
WHERE tenant_id = ? AND shift_id = ? AND invoice_id IS NULL;

-- name: RestampUnbilledShiftItems :exec
UPDATE line_items SET service_date = ?
WHERE tenant_id = ? AND shift_id = ? AND invoice_id IS NULL;

-- name: UnlinkShiftItemsFromInvoice :exec
UPDATE line_items SET invoice_id = NULL, sort_order = 0
WHERE tenant_id = ? AND invoice_id = ? AND shift_id IS NOT NULL;

-- name: DeleteLineItemsForInvoice :exec
DELETE FROM line_items WHERE tenant_id = ? AND invoice_id = ?;
