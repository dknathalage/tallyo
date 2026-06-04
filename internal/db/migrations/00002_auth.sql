-- +goose Up
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid          TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'member',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    last_login_at TEXT
);

CREATE TABLE invites (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    token      TEXT NOT NULL UNIQUE,
    email      TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'member',
    created_by INTEGER NOT NULL REFERENCES users(id),
    expires_at TEXT NOT NULL,
    used_at    TEXT
);
CREATE INDEX idx_invites_token ON invites (token);

-- scs sqlite3store session table (scs does NOT create this).
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
);
CREATE INDEX idx_sessions_expiry ON sessions (expiry);

-- +goose Down
DROP TABLE sessions;
DROP TABLE invites;
DROP TABLE users;
