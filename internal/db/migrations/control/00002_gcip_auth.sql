-- +goose Up
-- GCIP migration: move from password+scs-session auth to stateless Firebase
-- bearer-JWT auth. Pre-launch clean break — there is no data to preserve.
--   * users.password_hash is dropped (the backend never sees passwords).
--   * users.firebase_uid links the row to a Firebase identity; one uid may map
--     to many tenant rows (flat user pool, one row per (tenant, uid)).
--   * the scs sessions table is dropped (auth is now stateless).

ALTER TABLE users DROP COLUMN password_hash;
ALTER TABLE users ADD COLUMN firebase_uid TEXT NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_tenant_firebase_uid_key UNIQUE (tenant_id, firebase_uid);
CREATE INDEX idx_users_firebase_uid ON users (firebase_uid);

DROP TABLE sessions;

-- +goose Down
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BYTEA NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

DROP INDEX idx_users_firebase_uid;
ALTER TABLE users DROP CONSTRAINT users_tenant_firebase_uid_key;
ALTER TABLE users DROP COLUMN firebase_uid;
ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
