# Tallyo Data Model (ERD)

Living reference for the SQLite schema. Source of truth is the goose migrations
(`internal/db/migrations/*.sql`); this diagram is the human-readable map. Update
it whenever a migration changes a table or relationship.

> **Active change — shift items = invoice line items.** `line_items` is the single
> home for both a shift's items and an invoice's lines. A row is born on a shift
> (`shift_id` set, `invoice_id` NULL = unbilled); drafting an invoice sets its
> `invoice_id`. The row is never copied. `shifts` no longer carries `hours`/`km`/
> `measures` — every billable quantity is a `line_items` row whose `unit` class
> (time / distance / count) drives how its quantity is captured. A
> `CHECK (shift_id IS NOT NULL OR invoice_id IS NOT NULL)` forbids orphan rows.
> See `docs/superpowers/specs/2026-06-19-shift-items-unification-design.md`.

```mermaid
erDiagram
    tenants ||--o{ users : has
    tenants ||--o{ participants : has
    tenants ||--o{ invoices : has
    tenants ||--o{ shifts : has

    plan_managers |o--o{ participants : manages
    plan_managers |o--o{ invoices : "bills via"
    participants ||--o{ shifts : "supported in"
    participants ||--o{ invoices : "billed for"

    catalog_versions ||--o{ support_items : contains
    support_items ||--o{ support_item_prices : "priced by zone"

    invoices ||--o{ line_items : "lines (invoice_id)"
    shifts   ||--o{ line_items : "items (shift_id)"
    shifts   }o--o| invoices : "drafted into"
    support_items |o--o{ line_items : "catalogue source"
    custom_items  |o--o{ line_items : "custom source"
    catalog_versions |o--o{ line_items : "pinned version"

    invoices ||--o{ payments : "paid by"
    invoices ||--o{ estimates : "converted from"
    estimates ||--o{ estimate_line_items : lines
    participants |o--o{ recurring_templates : "auto-bills"

    line_items {
        int     id PK
        int     tenant_id FK
        int     shift_id   FK "ON DELETE CASCADE; NULL for manual/recurring lines"
        int     invoice_id FK "NULL = unbilled shift item"
        int     support_item_id FK "catalogue item (nullable)"
        int     custom_item_id  FK "custom item (nullable)"
        int     catalog_version_id FK "pinned price version"
        text    code "NDIS code snapshot"
        text    description "what was done (from shift note)"
        text    service_date
        text    unit "H / KM / EA / D / WK … drives input class"
        text    start_time "time-class units only"
        text    end_time   "time-class units only"
        real    quantity "derived (time/distance) or typed"
        real    unit_price "resolved from catalogue cap"
        int     gst_free
        real    line_total "quantity * unit_price"
        int     sort_order
    }

    shifts {
        int  id PK
        int  tenant_id FK
        int  participant_id FK
        text service_date
        text note "free text; AI or user divides into line_items"
        text tags "JSON array"
        text status "scheduled|recorded|drafted|sent|paid"
        int  invoice_id FK "set when drafted (lifecycle)"
        int  author_user_id FK
    }

    invoices {
        int  id PK
        int  tenant_id FK
        int  participant_id FK
        int  plan_manager_id FK
        text status
    }

    participants {
        int  id PK
        int  tenant_id FK
        int  plan_manager_id FK "NULL = self-managed"
    }
```

## Conventions

- Every tenant-owned table carries `tenant_id INTEGER NOT NULL REFERENCES tenants(id)`.
- `line_items` and `estimate_line_items` are near-identical shapes (invoice vs
  estimate); they are deliberately separate tables, not unified.
- Prices are pinned per line via `catalog_version_id` + `support_item_id` so an
  existing invoice is never re-priced when a newer catalogue version loads.
- Agent has **no persistent tables** (Smarts are one-shot). The `notes` table and
  all `agent_*` chat tables were dropped (migrations `00005`, `00007`).

## Tables not shown

Auth/infra and supporting tables omitted from the diagram for clarity:
`invites`, `sessions`, `business_profile`, `custom_items`, `tax_rates`,
`support_item_prices` (shown), `recurring_templates` (shown), `audit_log`.
