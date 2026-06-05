-- name: ListLineItems :many
SELECT * FROM line_items WHERE invoice_id = ? ORDER BY sort_order, id;

-- name: CreateLineItem :one
INSERT INTO line_items (uuid, invoice_id, description, quantity, rate, amount, notes, sort_order, catalog_item_id, rate_tier_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: DeleteLineItemsForInvoice :exec
DELETE FROM line_items WHERE invoice_id = ?;
