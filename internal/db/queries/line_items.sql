-- name: ListLineItemsForInvoice :many
SELECT * FROM line_items WHERE tenant_id = ? AND invoice_id = ? ORDER BY sort_order, id;

-- name: ListLineItemsForShift :many
SELECT * FROM line_items WHERE tenant_id = ? AND shift_id = ? ORDER BY id;

-- name: GetLineItem :one
SELECT * FROM line_items WHERE tenant_id = ? AND id = ?;

-- name: CreateLineItem :one
INSERT INTO line_items (
    uuid, tenant_id, shift_id, invoice_id, support_item_id, custom_item_id,
    catalog_version_id, code, description, service_date, unit, start_time,
    end_time, quantity, unit_price, gst_free, line_total, sort_order
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateShiftLineItem :one
UPDATE line_items SET
    support_item_id = ?, custom_item_id = ?, catalog_version_id = ?, code = ?,
    description = ?, service_date = ?, unit = ?, start_time = ?, end_time = ?,
    quantity = ?, unit_price = ?, gst_free = ?, line_total = ?
WHERE tenant_id = ? AND id = ? AND invoice_id IS NULL
RETURNING *;

-- name: DeleteShiftLineItem :exec
DELETE FROM line_items WHERE tenant_id = ? AND id = ? AND invoice_id IS NULL;

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
