-- +goose Up
-- Control-plane baseline (DB-per-tenant). Holds the global registry, auth, the
-- scs session store, the shared NDIS Support Catalogue (reference data), and the
-- global-admin audit log. Tenant business data lives in per-tenant files (see
-- migrations/tenant). Fresh clean-break schema — no data migration.

-- ---------------------------------------------------------------------------
-- Tenancy / auth
-- ---------------------------------------------------------------------------

CREATE TABLE tenants (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE users (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid              TEXT NOT NULL UNIQUE,
    tenant_id         INTEGER NOT NULL REFERENCES tenants(id),
    email             TEXT NOT NULL,
    password_hash     TEXT NOT NULL,
    name              TEXT NOT NULL DEFAULT '',
    is_platform_admin INTEGER NOT NULL DEFAULT 0,
    role              TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner','admin','member')),
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    last_login_at     TEXT,
    UNIQUE (tenant_id, email)
);
CREATE INDEX idx_users_tenant ON users (tenant_id);

CREATE TABLE invites (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid        TEXT NOT NULL UNIQUE,
    tenant_id   INTEGER NOT NULL REFERENCES tenants(id),
    token       TEXT NOT NULL UNIQUE,
    email       TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner','admin','member')),
    created_by  INTEGER NOT NULL REFERENCES users(id),
    expires_at  TEXT NOT NULL,
    accepted_at TEXT,
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_invites_token  ON invites (token);
CREATE INDEX idx_invites_tenant ON invites (tenant_id);

-- scs sqlite3store session table (scs does NOT create this).
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
);
CREATE INDEX idx_sessions_expiry ON sessions (expiry);

-- ---------------------------------------------------------------------------
-- Global NDIS Support Catalogue (shared reference data, NO tenant_id)
-- ---------------------------------------------------------------------------

CREATE TABLE catalog_versions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    label           TEXT NOT NULL,           -- e.g. '2025-26 v1.1'
    effective_from  TEXT NOT NULL,           -- DATE
    effective_to    TEXT,                    -- DATE NULL (open-ended = current)
    source_filename TEXT DEFAULT '',
    created_at      TEXT NOT NULL
);
CREATE INDEX idx_catalog_versions_effective ON catalog_versions (effective_from, effective_to);

CREATE TABLE support_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid               TEXT NOT NULL UNIQUE,
    catalog_version_id INTEGER NOT NULL REFERENCES catalog_versions(id) ON DELETE CASCADE,
    code               TEXT NOT NULL,
    name               TEXT NOT NULL,
    unit               TEXT DEFAULT '',
    support_category   TEXT DEFAULT '' CHECK (support_category IN ('Core','CB','Capital','')),
    registration_group TEXT DEFAULT '',
    claim_type         TEXT DEFAULT '',
    gst_free           INTEGER NOT NULL DEFAULT 1,
    metadata           TEXT DEFAULT '{}',
    UNIQUE (catalog_version_id, code)
);
CREATE INDEX idx_support_items_version ON support_items (catalog_version_id);
CREATE INDEX idx_support_items_code    ON support_items (code);

CREATE TABLE support_item_prices (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    support_item_id INTEGER NOT NULL REFERENCES support_items(id) ON DELETE CASCADE,
    zone            TEXT NOT NULL CHECK (zone IN ('national','remote','very_remote')),
    price_cap       REAL,                     -- NULL = quotable item (no fixed cap)
    UNIQUE (support_item_id, zone)
);
CREATE INDEX idx_support_item_prices_item ON support_item_prices (support_item_id);

-- ---------------------------------------------------------------------------
-- Global-admin audit log. The per-tenant files carry their OWN audit_log
-- (FKs dropped) — see migrations/tenant. Same column shape; only this copy
-- keeps the tenants/users FKs and is the one sqlc reads for the AuditLog model.
-- ---------------------------------------------------------------------------

CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid        TEXT NOT NULL UNIQUE,
    tenant_id   INTEGER REFERENCES tenants(id),
    user_id     INTEGER REFERENCES users(id),
    entity_type TEXT NOT NULL,
    entity_id   INTEGER,
    action      TEXT NOT NULL,
    changes     TEXT DEFAULT '{}',
    context     TEXT DEFAULT '',
    batch_id    TEXT,
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_audit_tenant  ON audit_log (tenant_id);
CREATE INDEX idx_audit_entity  ON audit_log (entity_type, entity_id);
CREATE INDEX idx_audit_batch   ON audit_log (batch_id);
CREATE INDEX idx_audit_created ON audit_log (created_at);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE support_item_prices;
DROP TABLE support_items;
DROP TABLE catalog_versions;
DROP TABLE sessions;
DROP TABLE invites;
DROP TABLE users;
DROP TABLE tenants;
