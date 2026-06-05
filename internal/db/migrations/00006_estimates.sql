-- +goose Up
CREATE TABLE estimates (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid                 TEXT NOT NULL UNIQUE,
    estimate_number      TEXT NOT NULL UNIQUE,
    client_id            INTEGER REFERENCES clients(id),
    date                 TEXT NOT NULL,
    valid_until          TEXT NOT NULL,
    subtotal             REAL DEFAULT 0,
    tax_rate             REAL DEFAULT 0,
    tax_rate_id          INTEGER REFERENCES tax_rates(id) ON DELETE SET NULL,
    tax_amount           REAL DEFAULT 0,
    total                REAL DEFAULT 0,
    notes                TEXT DEFAULT '',
    status               TEXT DEFAULT 'draft',
    currency_code        TEXT DEFAULT 'USD',
    converted_invoice_id INTEGER,
    business_snapshot    TEXT DEFAULT '{}',
    client_snapshot      TEXT DEFAULT '{}',
    payer_snapshot       TEXT DEFAULT '{}',
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);
CREATE INDEX idx_estimates_status ON estimates (status);
CREATE INDEX idx_estimates_client_id ON estimates (client_id);

CREATE TABLE estimate_line_items (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid            TEXT NOT NULL UNIQUE,
    estimate_id     INTEGER NOT NULL REFERENCES estimates(id) ON DELETE CASCADE,
    description     TEXT NOT NULL,
    quantity        REAL NOT NULL DEFAULT 1,
    rate            REAL NOT NULL DEFAULT 0,
    amount          REAL NOT NULL DEFAULT 0,
    notes           TEXT DEFAULT '',
    sort_order      INTEGER DEFAULT 0,
    catalog_item_id INTEGER,
    rate_tier_id    INTEGER
);
CREATE INDEX idx_estimate_line_items_estimate_id ON estimate_line_items (estimate_id);

-- +goose Down
DROP TABLE estimate_line_items;
DROP TABLE estimates;
