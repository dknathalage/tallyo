-- Shifts: the delivered-support unit (tenant-scoped). See migration 00004_shifts.sql.
-- Read queries LEFT JOIN participants for p.uuid so the slice DTO can expose the
-- participant FK as its uuid (participantId) instead of the internal int, and
-- LEFT JOIN invoices for i.uuid so the linked-invoice FK surfaces as its uuid
-- (invoiceId) instead of the internal int.

-- name: ListShifts :many
SELECT s.*, p.uuid AS participant_uuid, i.uuid AS invoice_uuid
FROM shifts s
LEFT JOIN participants p ON s.participant_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? ORDER BY s.service_date DESC, s.id DESC;

-- name: ListShiftsByParticipant :many
SELECT s.*, p.uuid AS participant_uuid, i.uuid AS invoice_uuid
FROM shifts s
LEFT JOIN participants p ON s.participant_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.participant_id = ?
ORDER BY s.service_date, s.id;

-- name: ListShiftsByParticipantRange :many
SELECT s.*, p.uuid AS participant_uuid, i.uuid AS invoice_uuid
FROM shifts s
LEFT JOIN participants p ON s.participant_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.participant_id = ?
  AND s.service_date >= ? AND s.service_date <= ?
ORDER BY s.service_date, s.id;

-- name: ListShiftsByStatus :many
SELECT s.*, p.uuid AS participant_uuid, i.uuid AS invoice_uuid
FROM shifts s
LEFT JOIN participants p ON s.participant_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.status = ? ORDER BY s.service_date, s.id;

-- name: ListScheduledShifts :many
SELECT s.*, p.uuid AS participant_uuid, i.uuid AS invoice_uuid
FROM shifts s
LEFT JOIN participants p ON s.participant_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.status = 'scheduled' ORDER BY s.service_date, s.id;

-- name: ListRecordedUnbilledByParticipant :many
SELECT s.*, p.uuid AS participant_uuid, i.uuid AS invoice_uuid
FROM shifts s
LEFT JOIN participants p ON s.participant_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.participant_id = ? AND s.status = 'recorded' AND s.invoice_id IS NULL
ORDER BY s.service_date, s.id;

-- name: GetShift :one
SELECT s.*, p.uuid AS participant_uuid, i.uuid AS invoice_uuid
FROM shifts s
LEFT JOIN participants p ON s.participant_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.uuid = ?;

-- name: GetShiftByID :one
SELECT s.*, p.uuid AS participant_uuid, i.uuid AS invoice_uuid
FROM shifts s
LEFT JOIN participants p ON s.participant_id = p.id AND p.tenant_id = s.tenant_id
LEFT JOIN invoices i ON s.invoice_id = i.id AND i.tenant_id = s.tenant_id
WHERE s.tenant_id = ? AND s.id = ?;

-- name: GetShiftIDByUUID :one
SELECT id FROM shifts WHERE tenant_id = ? AND uuid = ?;

-- name: CreateShift :one
INSERT INTO shifts (
    uuid, tenant_id, participant_id, service_date, note, tags, status,
    author_user_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateShift :one
UPDATE shifts SET
    service_date = ?, note = ?, tags = ?, status = ?, updated_at = ?
WHERE tenant_id = ? AND uuid = ?
RETURNING *;

-- name: UpdateShiftStatus :exec
UPDATE shifts SET status = ?, updated_at = ? WHERE tenant_id = ? AND uuid = ?;

-- name: SetShiftInvoice :exec
UPDATE shifts SET invoice_id = ?, status = ?, updated_at = ? WHERE tenant_id = ? AND id = ?;

-- name: SetStatusForInvoice :exec
UPDATE shifts SET status = ?, updated_at = ? WHERE tenant_id = ? AND invoice_id = ?;

-- name: ClearShiftsForInvoice :exec
UPDATE shifts SET invoice_id = NULL, status = 'recorded', updated_at = ?
WHERE tenant_id = ? AND invoice_id = ?;

-- name: DeleteShift :exec
DELETE FROM shifts WHERE tenant_id = ? AND uuid = ?;

-- name: ParticipantUnbilledAgg :many
SELECT participant_id, COUNT(*) AS cnt, MIN(service_date) AS from_date, MAX(service_date) AS to_date
FROM shifts
WHERE tenant_id = ? AND status = 'recorded' AND invoice_id IS NULL
GROUP BY participant_id;
