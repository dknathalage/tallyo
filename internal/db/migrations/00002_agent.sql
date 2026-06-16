-- +goose Up
CREATE TABLE agent_conversation (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  tenant_id   INTEGER NOT NULL,
  user_id     INTEGER NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  archived_at TEXT
);
CREATE INDEX idx_agent_conv_tenant ON agent_conversation(tenant_id, updated_at);

CREATE TABLE agent_message (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  conversation_id INTEGER NOT NULL REFERENCES agent_conversation(id) ON DELETE CASCADE,
  tenant_id       INTEGER NOT NULL,
  role            TEXT NOT NULL CHECK (role IN ('user','assistant')),
  content         TEXT NOT NULL,
  token_usage     TEXT NOT NULL DEFAULT '{}',
  created_at      TEXT NOT NULL
);
CREATE INDEX idx_agent_msg_conv ON agent_message(conversation_id, id);

CREATE TABLE agent_checkpoint (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id  INTEGER NOT NULL REFERENCES agent_message(id) ON DELETE CASCADE,
  tenant_id   INTEGER NOT NULL,
  status      TEXT NOT NULL CHECK (status IN ('open','committed','reverted')),
  created_at  TEXT NOT NULL,
  reverted_at TEXT
);

-- agent_step is created AFTER agent_checkpoint because it FKs it.
CREATE TABLE agent_step (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id   INTEGER NOT NULL REFERENCES agent_message(id) ON DELETE CASCADE,
  checkpoint_id INTEGER REFERENCES agent_checkpoint(id) ON DELETE SET NULL,
  tenant_id    INTEGER NOT NULL,
  ordinal      INTEGER NOT NULL,
  tool_name    TEXT NOT NULL,
  tool_use_id  TEXT NOT NULL DEFAULT '',
  summary      TEXT NOT NULL DEFAULT '',
  risk         TEXT NOT NULL CHECK (risk IN ('read','risky','meta')),
  status       TEXT NOT NULL CHECK (status IN ('planned','awaiting','allowed','denied','done','error')),
  pending_input TEXT NOT NULL DEFAULT '',
  result       TEXT NOT NULL DEFAULT '',
  await_expires_at TEXT,
  created_at   TEXT NOT NULL
);
CREATE INDEX idx_agent_step_msg ON agent_step(message_id, ordinal);
CREATE INDEX idx_agent_step_await ON agent_step(status, await_expires_at);

CREATE TABLE agent_checkpoint_change (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  checkpoint_id  INTEGER NOT NULL REFERENCES agent_checkpoint(id) ON DELETE CASCADE,
  tenant_id      INTEGER NOT NULL,
  ordinal        INTEGER NOT NULL,
  table_name     TEXT NOT NULL,
  pk             INTEGER NOT NULL,
  op             TEXT NOT NULL CHECK (op IN ('create','update')),
  before_row     TEXT,
  after_row      TEXT NOT NULL,
  entity_version TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL
);
CREATE INDEX idx_agent_chg_cp ON agent_checkpoint_change(checkpoint_id, ordinal);

CREATE TABLE agent_token_usage (
  tenant_id INTEGER NOT NULL,
  day       TEXT NOT NULL,
  tokens    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, day)
);

-- +goose Down
DROP TABLE agent_token_usage;
DROP TABLE agent_checkpoint_change;
DROP TABLE agent_step;
DROP TABLE agent_checkpoint;
DROP TABLE agent_message;
DROP TABLE agent_conversation;
