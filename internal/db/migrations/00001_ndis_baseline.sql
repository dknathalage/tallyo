-- +goose Up
-- Fresh NDIS-native multi-tenant baseline (pre-launch clean break).
-- Tenant-owned tables carry tenant_id INTEGER NOT NULL REFERENCES tenants(id).
-- Global NDIS Support Catalogue tables (catalog_versions, support_items,
-- support_item_prices) are shared reference data and carry NO tenant_id.

-- ---------------------------------------------------------------------------
-- 4.1 Tenancy / auth
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
-- 4.2 Tenant-owned business data (all carry tenant_id FK -> tenants)
-- ---------------------------------------------------------------------------

CREATE TABLE business_profile (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid             TEXT NOT NULL UNIQUE,
    tenant_id        INTEGER NOT NULL UNIQUE REFERENCES tenants(id),  -- 1:1 per tenant
    name             TEXT NOT NULL DEFAULT '',
    abn              TEXT DEFAULT '',
    email            TEXT DEFAULT '',
    phone            TEXT DEFAULT '',
    address          TEXT DEFAULT '',
    zone             TEXT NOT NULL DEFAULT 'national' CHECK (zone IN ('national','remote','very_remote')),
    logo             TEXT DEFAULT '',
    metadata         TEXT DEFAULT '{}',
    default_currency TEXT DEFAULT 'AUD',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

-- plan_managers (was payers)
CREATE TABLE plan_managers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    tenant_id  INTEGER NOT NULL REFERENCES tenants(id),
    name       TEXT NOT NULL,
    email      TEXT DEFAULT '',
    phone      TEXT DEFAULT '',
    address    TEXT DEFAULT '',
    metadata   TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_plan_managers_tenant ON plan_managers (tenant_id);

-- participants (was clients)
CREATE TABLE participants (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id),
    name            TEXT NOT NULL,
    ndis_number     TEXT DEFAULT '',
    plan_start      TEXT,                                    -- DATE
    plan_end        TEXT,                                    -- DATE
    mgmt_type       TEXT NOT NULL DEFAULT 'plan' CHECK (mgmt_type IN ('plan','self')),
    plan_manager_id INTEGER REFERENCES plan_managers(id) ON DELETE SET NULL,  -- NULL when self-managed
    email           TEXT DEFAULT '',
    phone           TEXT DEFAULT '',
    address         TEXT DEFAULT '',
    metadata        TEXT DEFAULT '{}',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX idx_participants_tenant       ON participants (tenant_id);
CREATE INDEX idx_participants_plan_manager ON participants (plan_manager_id);

CREATE TABLE custom_items (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    tenant_id  INTEGER NOT NULL REFERENCES tenants(id),
    name       TEXT NOT NULL,
    rate       REAL NOT NULL DEFAULT 0,
    unit       TEXT DEFAULT '',
    gst_free   INTEGER NOT NULL DEFAULT 0,
    metadata   TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_custom_items_tenant ON custom_items (tenant_id);

CREATE TABLE tax_rates (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    tenant_id  INTEGER NOT NULL REFERENCES tenants(id),
    name       TEXT NOT NULL,
    rate       REAL NOT NULL DEFAULT 0,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_tax_rates_tenant ON tax_rates (tenant_id);

-- ---------------------------------------------------------------------------
-- 4.3 Global NDIS Support Catalogue (NO tenant_id)
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
-- 4.2 (cont.) Invoices / estimates / payments / recurring / audit
-- ---------------------------------------------------------------------------

CREATE TABLE invoices (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid              TEXT NOT NULL UNIQUE,
    tenant_id         INTEGER NOT NULL REFERENCES tenants(id),
    number            TEXT NOT NULL,
    participant_id    INTEGER NOT NULL REFERENCES participants(id),
    plan_manager_id   INTEGER REFERENCES plan_managers(id) ON DELETE SET NULL,
    status            TEXT NOT NULL DEFAULT 'draft',
    issue_date        TEXT NOT NULL,
    due_date          TEXT NOT NULL,
    subtotal          REAL NOT NULL DEFAULT 0,
    tax               REAL NOT NULL DEFAULT 0,
    total             REAL NOT NULL DEFAULT 0,
    notes             TEXT DEFAULT '',
    business_snapshot TEXT DEFAULT '{}',
    client_snapshot   TEXT DEFAULT '{}',
    payer_snapshot    TEXT DEFAULT '{}',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    UNIQUE (tenant_id, number)
);
CREATE INDEX idx_invoices_tenant      ON invoices (tenant_id);
CREATE INDEX idx_invoices_status      ON invoices (status);
CREATE INDEX idx_invoices_participant ON invoices (participant_id);
CREATE INDEX idx_invoices_created_at  ON invoices (created_at);

CREATE TABLE line_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid               TEXT NOT NULL UNIQUE,
    tenant_id          INTEGER NOT NULL REFERENCES tenants(id),
    invoice_id         INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    support_item_id    INTEGER REFERENCES support_items(id) ON DELETE SET NULL,
    custom_item_id     INTEGER REFERENCES custom_items(id) ON DELETE SET NULL,
    catalog_version_id INTEGER REFERENCES catalog_versions(id) ON DELETE SET NULL,  -- pinned
    code               TEXT DEFAULT '',     -- snapshot
    description        TEXT NOT NULL,       -- snapshot
    service_date       TEXT,                -- DATE
    unit               TEXT DEFAULT '',
    quantity           REAL NOT NULL DEFAULT 1,
    unit_price         REAL NOT NULL DEFAULT 0,
    gst_free           INTEGER NOT NULL DEFAULT 0,
    line_total         REAL NOT NULL DEFAULT 0,
    sort_order         INTEGER DEFAULT 0
);
CREATE INDEX idx_line_items_tenant       ON line_items (tenant_id);
CREATE INDEX idx_line_items_invoice      ON line_items (invoice_id);
CREATE INDEX idx_line_items_support_item ON line_items (support_item_id);

CREATE TABLE estimates (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid                 TEXT NOT NULL UNIQUE,
    tenant_id            INTEGER NOT NULL REFERENCES tenants(id),
    number               TEXT NOT NULL,
    participant_id       INTEGER REFERENCES participants(id),
    plan_manager_id      INTEGER REFERENCES plan_managers(id) ON DELETE SET NULL,
    status               TEXT NOT NULL DEFAULT 'draft',
    issue_date           TEXT NOT NULL,
    valid_until          TEXT NOT NULL,
    subtotal             REAL NOT NULL DEFAULT 0,
    tax                  REAL NOT NULL DEFAULT 0,
    total                REAL NOT NULL DEFAULT 0,
    notes                TEXT DEFAULT '',
    converted_invoice_id INTEGER REFERENCES invoices(id) ON DELETE SET NULL,
    business_snapshot    TEXT DEFAULT '{}',
    client_snapshot      TEXT DEFAULT '{}',
    payer_snapshot       TEXT DEFAULT '{}',
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    UNIQUE (tenant_id, number)
);
CREATE INDEX idx_estimates_tenant      ON estimates (tenant_id);
CREATE INDEX idx_estimates_status      ON estimates (status);
CREATE INDEX idx_estimates_participant ON estimates (participant_id);

CREATE TABLE estimate_line_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid               TEXT NOT NULL UNIQUE,
    tenant_id          INTEGER NOT NULL REFERENCES tenants(id),
    estimate_id        INTEGER NOT NULL REFERENCES estimates(id) ON DELETE CASCADE,
    support_item_id    INTEGER REFERENCES support_items(id) ON DELETE SET NULL,
    custom_item_id     INTEGER REFERENCES custom_items(id) ON DELETE SET NULL,
    catalog_version_id INTEGER REFERENCES catalog_versions(id) ON DELETE SET NULL,  -- pinned
    code               TEXT DEFAULT '',     -- snapshot
    description        TEXT NOT NULL,       -- snapshot
    service_date       TEXT,                -- DATE
    unit               TEXT DEFAULT '',
    quantity           REAL NOT NULL DEFAULT 1,
    unit_price         REAL NOT NULL DEFAULT 0,
    gst_free           INTEGER NOT NULL DEFAULT 0,
    line_total         REAL NOT NULL DEFAULT 0,
    sort_order         INTEGER DEFAULT 0
);
CREATE INDEX idx_estimate_line_items_tenant   ON estimate_line_items (tenant_id);
CREATE INDEX idx_estimate_line_items_estimate ON estimate_line_items (estimate_id);

CREATE TABLE payments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    tenant_id  INTEGER NOT NULL REFERENCES tenants(id),
    invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    amount     REAL NOT NULL,
    paid_at    TEXT NOT NULL,
    method     TEXT DEFAULT '',
    reference  TEXT DEFAULT '',
    notes      TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_payments_tenant  ON payments (tenant_id);
CREATE INDEX idx_payments_invoice ON payments (invoice_id);

CREATE TABLE recurring_templates (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    tenant_id       INTEGER NOT NULL REFERENCES tenants(id),
    participant_id  INTEGER REFERENCES participants(id) ON DELETE SET NULL,
    plan_manager_id INTEGER REFERENCES plan_managers(id) ON DELETE SET NULL,
    name            TEXT NOT NULL,
    frequency       TEXT NOT NULL,
    next_due        TEXT NOT NULL,
    line_items      TEXT NOT NULL DEFAULT '[]',  -- NDIS-aware line template (JSON)
    tax_rate        REAL NOT NULL DEFAULT 0,
    notes           TEXT NOT NULL DEFAULT '',
    is_active       INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX idx_recurring_tenant      ON recurring_templates (tenant_id);
CREATE INDEX idx_recurring_participant ON recurring_templates (participant_id);
CREATE INDEX idx_recurring_next_due    ON recurring_templates (next_due);

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
DROP TABLE recurring_templates;
DROP TABLE payments;
DROP TABLE estimate_line_items;
DROP TABLE estimates;
DROP TABLE line_items;
DROP TABLE invoices;
DROP TABLE support_item_prices;
DROP TABLE support_items;
DROP TABLE catalog_versions;
DROP TABLE tax_rates;
DROP TABLE custom_items;
DROP TABLE participants;
DROP TABLE plan_managers;
DROP TABLE business_profile;
DROP TABLE sessions;
DROP TABLE invites;
DROP TABLE users;
DROP TABLE tenants;
