-- name: CreateAgentConversation :one
INSERT INTO agent_conversation (tenant_id, user_id, title, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAgentConversation :one
SELECT * FROM agent_conversation
WHERE tenant_id = ? AND id = ?;

-- name: GetConversationByMessage :one
SELECT c.* FROM agent_conversation c
JOIN agent_message m ON m.conversation_id = c.id AND m.tenant_id = c.tenant_id
WHERE c.tenant_id = ? AND m.id = ?;

-- name: ListAgentConversations :many
SELECT * FROM agent_conversation
WHERE tenant_id = ?
ORDER BY updated_at DESC;

-- name: TouchAgentConversation :exec
UPDATE agent_conversation SET updated_at = ?
WHERE tenant_id = ? AND id = ?;

-- name: CreateAgentMessage :one
INSERT INTO agent_message (conversation_id, tenant_id, role, content, token_usage, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListAgentMessages :many
SELECT * FROM agent_message
WHERE tenant_id = ? AND conversation_id = ?
ORDER BY id ASC;

-- name: CreateAgentStep :one
INSERT INTO agent_step (
    message_id, checkpoint_id, tenant_id, ordinal, tool_name, tool_use_id,
    summary, risk, status, pending_input, result, await_expires_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: CreateAwaitingStep :one
INSERT INTO agent_step (
    message_id, checkpoint_id, tenant_id, ordinal, tool_name, tool_use_id,
    summary, risk, status, pending_input, result, await_expires_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'awaiting', ?, '', ?, ?)
RETURNING *;

-- name: UpdateAgentStepStatus :exec
UPDATE agent_step SET status = ?, result = ?
WHERE tenant_id = ? AND id = ?;

-- name: GetAgentStep :one
SELECT * FROM agent_step
WHERE tenant_id = ? AND id = ?;

-- name: ListAgentSteps :many
SELECT * FROM agent_step
WHERE tenant_id = ? AND message_id = ?
ORDER BY ordinal ASC;

-- name: ListExpiredAwaitingSteps :many
SELECT * FROM agent_step
WHERE status = 'awaiting' AND await_expires_at IS NOT NULL AND await_expires_at < ?
ORDER BY id ASC;

-- name: CreateCheckpoint :one
INSERT INTO agent_checkpoint (message_id, tenant_id, status, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateCheckpointStatus :exec
UPDATE agent_checkpoint SET status = ?
WHERE tenant_id = ? AND id = ?;

-- name: MarkCheckpointReverted :exec
UPDATE agent_checkpoint SET status = 'reverted', reverted_at = ?
WHERE tenant_id = ? AND id = ?;

-- name: GetCheckpoint :one
SELECT * FROM agent_checkpoint
WHERE tenant_id = ? AND id = ?;

-- name: CreateCheckpointChange :one
-- ordinal is auto-assigned as the next value per checkpoint (atomic within the
-- insert) so callers never have to track it; reverse-ordinal replay in Revert
-- is therefore correct across a multi-step turn.
INSERT INTO agent_checkpoint_change (
    checkpoint_id, tenant_id, ordinal, table_name, pk, op,
    before_row, after_row, entity_version, created_at
) VALUES (
    @checkpoint_id, @tenant_id,
    (SELECT COALESCE(MAX(ordinal), 0) + 1 FROM agent_checkpoint_change WHERE checkpoint_id = @checkpoint_id),
    @table_name, @pk, @op, @before_row, @after_row, @entity_version, @created_at
)
RETURNING *;

-- name: ListCheckpointChanges :many
SELECT * FROM agent_checkpoint_change
WHERE tenant_id = ? AND checkpoint_id = ?
ORDER BY ordinal DESC, id DESC;

-- name: AddTokenUsage :exec
INSERT INTO agent_token_usage (tenant_id, day, tokens)
VALUES (?, ?, ?)
ON CONFLICT(tenant_id, day) DO UPDATE SET tokens = tokens + excluded.tokens;

-- name: GetTokenUsage :one
SELECT CAST(COALESCE((SELECT tokens FROM agent_token_usage WHERE tenant_id = ? AND day = ?), 0) AS INTEGER) AS tokens;

-- name: PruneCheckpointChanges :exec
DELETE FROM agent_checkpoint_change WHERE created_at < ?;

-- name: PruneAgentSteps :exec
DELETE FROM agent_step WHERE created_at < ?;
