-- name: ListLineItems :many
SELECT * FROM line_items WHERE tenant_id = ? AND invoice_id = ? ORDER BY sort_order, id;

-- name: CreateLineItem :one
INSERT INTO line_items (
    uuid, tenant_id, invoice_id, support_item_id, custom_item_id, catalog_version_id,
    code, description, service_date, unit, quantity, unit_price, gst_free, line_total, sort_order
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteLineItemsForInvoice :exec
DELETE FROM line_items WHERE tenant_id = ? AND invoice_id = ?;
