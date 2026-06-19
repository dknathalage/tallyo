-- +goose Up
-- +goose StatementBegin

-- Unify a shift's items with invoice line items into one line_items row, and drop
-- the shift's structured quantity columns (billable quantity now lives on the
-- shift's line_items rows). Both tables need a rebuild — SQLite can't drop a
-- NOT NULL, drop columns, or add a CHECK in place.
--
-- Order matters: rebuild `shifts` FIRST. Nothing references shifts(id) today, so
-- its drop is safe; the new line_items.shift_id FK (added second) then points at
-- the already-rebuilt shifts. Doing line_items first would drop `shifts` while a
-- child FK references it.

-- shifts: drop hours/km/measures/start_time/end_time.
CREATE TABLE shifts_new (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid           TEXT NOT NULL UNIQUE,
    tenant_id      INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    participant_id INTEGER NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    service_date   TEXT NOT NULL,
    note           TEXT NOT NULL DEFAULT '',
    tags           TEXT NOT NULL DEFAULT '[]',
    status         TEXT NOT NULL DEFAULT 'recorded'
                     CHECK (status IN ('scheduled','recorded','drafted','sent','paid')),
    invoice_id     INTEGER REFERENCES invoices(id) ON DELETE SET NULL,
    author_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
INSERT INTO shifts_new (id, uuid, tenant_id, participant_id, service_date, note,
    tags, status, invoice_id, author_user_id, created_at, updated_at)
SELECT id, uuid, tenant_id, participant_id, service_date, note, tags, status,
    invoice_id, author_user_id, created_at, updated_at
FROM shifts;
DROP TABLE shifts;
ALTER TABLE shifts_new RENAME TO shifts;
CREATE INDEX idx_shifts_participant_date ON shifts(tenant_id, participant_id, service_date);
CREATE INDEX idx_shifts_status ON shifts(tenant_id, status);
CREATE INDEX idx_shifts_invoice ON shifts(invoice_id);

-- line_items: invoice_id becomes nullable; add shift_id (ON DELETE CASCADE),
-- start/end time, and a no-orphan CHECK.
CREATE TABLE line_items_new (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid               TEXT NOT NULL UNIQUE,
    tenant_id          INTEGER NOT NULL REFERENCES tenants(id),
    shift_id           INTEGER REFERENCES shifts(id) ON DELETE CASCADE,
    invoice_id         INTEGER REFERENCES invoices(id) ON DELETE CASCADE,
    support_item_id    INTEGER REFERENCES support_items(id) ON DELETE SET NULL,
    custom_item_id     INTEGER REFERENCES custom_items(id) ON DELETE SET NULL,
    catalog_version_id INTEGER REFERENCES catalog_versions(id) ON DELETE SET NULL,
    code               TEXT DEFAULT '',
    description        TEXT NOT NULL,
    service_date       TEXT,
    unit               TEXT DEFAULT '',
    start_time         TEXT,
    end_time           TEXT,
    quantity           REAL NOT NULL DEFAULT 1,
    unit_price         REAL NOT NULL DEFAULT 0,
    gst_free           INTEGER NOT NULL DEFAULT 0,
    line_total         REAL NOT NULL DEFAULT 0,
    sort_order         INTEGER DEFAULT 0,
    CHECK (shift_id IS NOT NULL OR invoice_id IS NOT NULL)
);
INSERT INTO line_items_new (
    id, uuid, tenant_id, shift_id, invoice_id, support_item_id, custom_item_id,
    catalog_version_id, code, description, service_date, unit, quantity,
    unit_price, gst_free, line_total, sort_order)
SELECT id, uuid, tenant_id, NULL, invoice_id, support_item_id, custom_item_id,
    catalog_version_id, code, description, service_date, unit, quantity,
    unit_price, gst_free, line_total, sort_order
FROM line_items;
DROP TABLE line_items;
ALTER TABLE line_items_new RENAME TO line_items;
-- recreate all three original indexes (00001: tenant + invoice + support_item)
-- plus the new shift index. Column DEFAULTs from 00001 are preserved (some tests
-- + future callers do implicit inserts).
CREATE INDEX idx_line_items_tenant       ON line_items(tenant_id);
CREATE INDEX idx_line_items_invoice      ON line_items(invoice_id);
CREATE INDEX idx_line_items_support_item ON line_items(support_item_id);
CREATE INDEX idx_line_items_shift        ON line_items(shift_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT RAISE(FAIL, 'irreversible: shift hours/km/measures dropped, no backfill');
-- +goose StatementEnd
