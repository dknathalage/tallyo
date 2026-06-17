-- +goose Up
-- notes: a per-participant daily journal kept by the provider. The AI reads a
-- date range of these to draft an invoice. body is free-text (UNTRUSTED when
-- fed to the model); transport_km / support_hours are optional structured tags
-- the provider may jot down. billed_invoice_id is a SOFT billing flag (nullable,
-- cleared if the invoice is deleted) — it never blocks editing or re-billing.
CREATE TABLE notes (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  uuid              TEXT NOT NULL UNIQUE,
  tenant_id         INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  participant_id    INTEGER NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  service_date      TEXT NOT NULL,
  body              TEXT NOT NULL,
  transport_km      REAL,
  support_hours     REAL,
  author_user_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  billed_invoice_id INTEGER REFERENCES invoices(id) ON DELETE SET NULL,
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
);
CREATE INDEX idx_notes_participant_date ON notes(tenant_id, participant_id, service_date);
CREATE INDEX idx_notes_billed ON notes(billed_invoice_id);

-- +goose Down
DROP TABLE notes;
