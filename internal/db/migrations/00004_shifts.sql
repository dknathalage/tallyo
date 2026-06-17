-- +goose Up
-- shifts: the first-class unit of delivered support, superseding notes. A shift
-- is semi-structured (time/participant structured; note is free text; measures &
-- tags are JSON). It carries a lifecycle status: scheduled → recorded → drafted
-- → sent → paid. Added ALONGSIDE notes; notes is removed in a later migration
-- once all consumers are migrated.
CREATE TABLE shifts (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  uuid           TEXT NOT NULL UNIQUE,
  tenant_id      INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  participant_id INTEGER NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  service_date   TEXT NOT NULL,
  start_time     TEXT NOT NULL DEFAULT '',
  end_time       TEXT NOT NULL DEFAULT '',
  hours          REAL NOT NULL DEFAULT 0,
  km             REAL NOT NULL DEFAULT 0,
  measures       TEXT NOT NULL DEFAULT '[]',
  note           TEXT NOT NULL DEFAULT '',
  tags           TEXT NOT NULL DEFAULT '[]',
  status         TEXT NOT NULL DEFAULT 'recorded'
                   CHECK (status IN ('scheduled','recorded','drafted','sent','paid')),
  invoice_id     INTEGER REFERENCES invoices(id) ON DELETE SET NULL,
  author_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL
);
CREATE INDEX idx_shifts_participant_date ON shifts(tenant_id, participant_id, service_date);
CREATE INDEX idx_shifts_status ON shifts(tenant_id, status);
CREATE INDEX idx_shifts_invoice ON shifts(invoice_id);

-- +goose Down
DROP TABLE shifts;
