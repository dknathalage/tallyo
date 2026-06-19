-- +goose Up
-- The conversational AI agent harness has been removed; its tables are now dead.
-- Drop children before parents (FKs are internal to the agent_* cluster and
-- foreign_keys=ON). No prod data to preserve (clean-break).
DROP TABLE IF EXISTS agent_checkpoint_change;
DROP TABLE IF EXISTS agent_step;
DROP TABLE IF EXISTS agent_checkpoint;
DROP TABLE IF EXISTS agent_message;
DROP TABLE IF EXISTS agent_conversation;
DROP TABLE IF EXISTS agent_token_usage;

-- +goose Down
-- Clean-break: the agent chat schema is not restored. To recover the schema,
-- see 00002_agent.sql (the original CREATE TABLE statements + indexes).
SELECT 1;
