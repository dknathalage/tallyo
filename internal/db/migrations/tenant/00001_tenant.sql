-- +goose Up
-- Per-tenant baseline (DB-per-tenant). One of these files exists per tenant
-- (tenants/tenant-<id>.db). Holds only that tenant's business data. Fresh
-- clean-break schema reflecting the FINAL state after the old 00001..00008
-- sequence (agent_* and notes were dropped; sessions + line_items unified).
--
-- Cross-DB references are NOT foreign keys (the target tables live in
-- control.db): `tenant_id` is a plain guard column; catalogue links
-- (support_item_id, catalog_version_id) are stored as the control-DB UUID
-- (TEXT) and validated in app; user links (author_user_id) are non-authoritative
-- control ids. Same-file FKs are kept.

CREATE TABLE payers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    tenant_id  INTEGER NOT NULL,
    name       TEXT NOT NULL,
    email      TEXT DEFAULT '',
    phone      TEXT DEFAULT '',
    address    TEXT DEFAULT '',
    metadata   TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_payers_tenant ON payers (tenant_id);

CREATE TABLE clients (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    tenant_id       INTEGER NOT NULL,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL DEFAULT 'standard' CHECK (type IN ('ndis','standard')),
    reference       TEXT DEFAULT '',
    plan_start      TEXT,                                    -- DATE
    plan_end        TEXT,                                    -- DATE
    mgmt_type       TEXT CHECK (mgmt_type IN ('plan','self')),
    payer_id INTEGER REFERENCES payers(id) ON DELETE SET NULL,
    email           TEXT DEFAULT '',
    phone           TEXT DEFAULT '',
    address         TEXT DEFAULT '',
    metadata        TEXT DEFAULT '{}',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX idx_clients_tenant       ON clients (tenant_id);
CREATE INDEX idx_clients_payer ON clients (payer_id);

CREATE TABLE business_profile (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid             TEXT NOT NULL UNIQUE,
    tenant_id        INTEGER NOT NULL UNIQUE,   -- 1:1 per tenant (file scopes it)
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

CREATE TABLE custom_items (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    tenant_id  INTEGER NOT NULL,
    name       TEXT NOT NULL,
    rate       REAL NOT NULL DEFAULT 0,
    unit       TEXT DEFAULT '',
    taxable    INTEGER NOT NULL DEFAULT 1,
    metadata   TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_custom_items_tenant ON custom_items (tenant_id);

CREATE TABLE tax_rates (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    tenant_id  INTEGER NOT NULL,
    name       TEXT NOT NULL,
    rate       REAL NOT NULL DEFAULT 0,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_tax_rates_tenant ON tax_rates (tenant_id);

CREATE TABLE invoices (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid              TEXT NOT NULL UNIQUE,
    tenant_id         INTEGER NOT NULL,
    number            TEXT NOT NULL,
    client_id         INTEGER NOT NULL REFERENCES clients(id),
    payer_id   INTEGER REFERENCES payers(id) ON DELETE SET NULL,
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
CREATE INDEX idx_invoices_client ON invoices (client_id);
CREATE INDEX idx_invoices_created_at  ON invoices (created_at);

-- sessions: final shape (00008) — no hours/km/measures/start_time/end_time.
-- author_user_id references control-DB users: column kept, FK dropped.
CREATE TABLE sessions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid           TEXT NOT NULL UNIQUE,
    tenant_id      INTEGER NOT NULL,
    client_id      INTEGER NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    service_date   TEXT NOT NULL,
    note           TEXT NOT NULL DEFAULT '',
    tags           TEXT NOT NULL DEFAULT '[]',
    status         TEXT NOT NULL DEFAULT 'recorded'
                     CHECK (status IN ('scheduled','recorded','drafted','sent','paid')),
    invoice_id     INTEGER REFERENCES invoices(id) ON DELETE SET NULL,
    author_user_id INTEGER,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX idx_sessions_client_date ON sessions(tenant_id, client_id, service_date);
CREATE INDEX idx_sessions_status          ON sessions(tenant_id, status);
CREATE INDEX idx_sessions_invoice         ON sessions(invoice_id);

-- line_items: final shape (00008) — session_id + invoice_id (one required),
-- start/end time. Catalogue links are control-DB UUIDs (TEXT, no FK).
CREATE TABLE line_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid               TEXT NOT NULL UNIQUE,
    tenant_id          INTEGER NOT NULL,
    session_id           INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    invoice_id         INTEGER REFERENCES invoices(id) ON DELETE CASCADE,
    support_item_id    TEXT DEFAULT '',     -- control-DB support_items.uuid (no FK)
    custom_item_id     INTEGER REFERENCES custom_items(id) ON DELETE SET NULL,
    catalog_version_id TEXT DEFAULT '',     -- control-DB catalog_versions.uuid (no FK), pinned
    code               TEXT DEFAULT '',     -- snapshot
    description        TEXT NOT NULL,       -- snapshot
    service_date       TEXT,                -- DATE
    unit               TEXT DEFAULT '',
    start_time         TEXT,
    end_time           TEXT,
    quantity           REAL NOT NULL DEFAULT 1,
    unit_price         REAL NOT NULL DEFAULT 0,
    taxable            INTEGER NOT NULL DEFAULT 1,
    line_total         REAL NOT NULL DEFAULT 0,
    sort_order         INTEGER DEFAULT 0,
    CHECK (session_id IS NOT NULL OR invoice_id IS NOT NULL)
);
CREATE INDEX idx_line_items_tenant       ON line_items(tenant_id);
CREATE INDEX idx_line_items_invoice      ON line_items(invoice_id);
CREATE INDEX idx_line_items_support_item ON line_items(support_item_id);
CREATE INDEX idx_line_items_session        ON line_items(session_id);

CREATE TABLE estimates (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid                 TEXT NOT NULL UNIQUE,
    tenant_id            INTEGER NOT NULL,
    number               TEXT NOT NULL,
    client_id            INTEGER REFERENCES clients(id),
    payer_id      INTEGER REFERENCES payers(id) ON DELETE SET NULL,
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
CREATE INDEX idx_estimates_client ON estimates (client_id);

CREATE TABLE estimate_line_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid               TEXT NOT NULL UNIQUE,
    tenant_id          INTEGER NOT NULL,
    estimate_id        INTEGER NOT NULL REFERENCES estimates(id) ON DELETE CASCADE,
    support_item_id    TEXT DEFAULT '',     -- control-DB support_items.uuid (no FK)
    custom_item_id     INTEGER REFERENCES custom_items(id) ON DELETE SET NULL,
    catalog_version_id TEXT DEFAULT '',     -- control-DB catalog_versions.uuid (no FK), pinned
    code               TEXT DEFAULT '',     -- snapshot
    description        TEXT NOT NULL,       -- snapshot
    service_date       TEXT,                -- DATE
    unit               TEXT DEFAULT '',
    quantity           REAL NOT NULL DEFAULT 1,
    unit_price         REAL NOT NULL DEFAULT 0,
    taxable            INTEGER NOT NULL DEFAULT 1,
    line_total         REAL NOT NULL DEFAULT 0,
    sort_order         INTEGER DEFAULT 0
);
CREATE INDEX idx_estimate_line_items_tenant   ON estimate_line_items (tenant_id);
CREATE INDEX idx_estimate_line_items_estimate ON estimate_line_items (estimate_id);

CREATE TABLE payments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    tenant_id  INTEGER NOT NULL,
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
    tenant_id       INTEGER NOT NULL,
    client_id       INTEGER REFERENCES clients(id) ON DELETE SET NULL,
    payer_id INTEGER REFERENCES payers(id) ON DELETE SET NULL,
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
CREATE INDEX idx_recurring_client ON recurring_templates (client_id);
CREATE INDEX idx_recurring_next_due    ON recurring_templates (next_due);

-- +goose Down
DROP TABLE recurring_templates;
DROP TABLE payments;
DROP TABLE estimate_line_items;
DROP TABLE estimates;
DROP TABLE line_items;
DROP TABLE sessions;
DROP TABLE invoices;
DROP TABLE tax_rates;
DROP TABLE custom_items;
DROP TABLE business_profile;
DROP TABLE clients;
DROP TABLE payers;
