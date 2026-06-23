# Tallyo Data Model (ERD)

Living reference for the SQLite schema. Source of truth is the goose migrations
(`internal/db/migrations/{control,tenant}/*.sql`); this diagram is the
human-readable map. Update it whenever a migration changes a table or relationship.

> **Single SQLite file, logical tenancy.** All tables live in one file
> (`<data-dir>/tallyo.db`). Conceptually they still group into a **control** set
> (`tenants, users, invites, sessions`, a global `audit_log`) and **tenant**
> business tables below — including the **tenant-owned price list**
> (`price_list_versions, items`, each tenant populates its own) — but that split
> is logical only; there is one physical database. Tenancy is enforced by a
> `tenant_id` column on every business row plus a `WHERE tenant_id = ?` guard on
> every query. `tenant_id` (→ `tenants`) and `author_user_id` / `user_id` (→
> `users`) are **logical references — NOT foreign keys** (validated in app).
> `item_id` / `price_list_version_id` reference the price list stored as **UUID
> TEXT** (not FKs — pinned per line so old invoices never re-price). (The model
> was simplified from an earlier DB-per-tenant design; that historical spec lives
> under `docs/superpowers/specs/`.)

> **Session items = invoice line items.** `line_items` is the single home for both
> a work session's items and an invoice's lines. A row is born on a session
> (`session_id` set, `invoice_id` NULL = unbilled); drafting an invoice sets its
> `invoice_id`. The row is never copied. The tenant table is `work_sessions` (gen
> model `WorkSession`, mapped to domain `Session` — named `work_sessions` to avoid
> colliding with the scs `sessions` table in the same DB when sqlc merges both
> schemas); it carries no `hours`/`km`/`measures` — every billable quantity is a
> `line_items` row whose `unit` class (time / distance / count) drives how its
> quantity is captured. A `CHECK (session_id IS NOT NULL OR invoice_id IS NOT NULL)`
> forbids orphan rows.
> See `docs/superpowers/specs/2026-06-19-shift-items-unification-design.md`.

```mermaid
erDiagram
    tenants ||--o{ users : has
    tenants ||--o{ clients : has
    tenants ||--o{ invoices : has
    tenants ||--o{ work_sessions : has

    payers |o--o{ clients : manages
    payers |o--o{ invoices : "bills via"
    clients ||--o{ work_sessions : "supported in"
    clients ||--o{ invoices : "billed for"

    price_list_versions ||--o{ items : contains

    invoices ||--o{ line_items : "lines (invoice_id)"
    work_sessions ||--o{ line_items : "items (session_id)"
    work_sessions }o--o| invoices : "drafted into"
    items |o--o{ line_items : "price-list source"
    custom_items  |o--o{ line_items : "custom source"
    price_list_versions |o--o{ line_items : "pinned version"

    invoices ||--o{ payments : "paid by"
    invoices ||--o{ estimates : "converted from"
    estimates ||--o{ estimate_line_items : lines
    clients |o--o{ recurring_templates : "auto-bills"

    line_items {
        int     id PK
        int     tenant_id FK
        int     session_id FK "→ work_sessions; ON DELETE CASCADE; NULL for manual/recurring lines"
        int     invoice_id FK "NULL = unbilled session item"
        text    item_id "tenant items.uuid (TEXT, no FK)"
        int     custom_item_id  FK "custom item (tenant-local, nullable)"
        text    price_list_version_id "tenant price_list_versions.uuid (TEXT, no FK), pinned"
        text    code "item code snapshot"
        text    description "what was done (from session note)"
        text    service_date
        text    unit "H / KM / EA / D / WK … drives input class"
        text    start_time "time-class units only"
        text    end_time   "time-class units only"
        real    quantity "derived (time/distance) or typed"
        real    unit_price "resolved from price list (items.unit_price)"
        int     taxable "1 = taxable"
        real    line_total "quantity * unit_price"
        int     sort_order
    }

    work_sessions {
        int  id PK
        int  tenant_id FK
        int  client_id FK
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
        int  client_id FK
        int  payer_id FK "NULL = self-managed"
        text status
    }

    clients {
        int  id PK
        int  tenant_id FK
        text name
        text reference "free-text, nullable"
        int  payer_id FK "NULL = self-billed"
    }

    items {
        int  id PK
        int  price_list_version_id FK "→ price_list_versions"
        text code
        text name
        text unit
        text category "nullable"
        real unit_price "generic base price"
        int  taxable
    }
```

## Conventions

- Every tenant-owned table carries a `tenant_id INTEGER` column — the scoping
  guard that every query filters on (`WHERE tenant_id = ?`). It is NOT a foreign
  key (validated in app).
- `line_items` and `estimate_line_items` are near-identical shapes (invoice vs
  estimate); they are deliberately separate tables, not unified.
- The price list (`price_list_versions`, `items`)
  is **tenant-owned** — scoped per tenant by `tenant_id` in the single DB, each
  tenant populating its own rows. `items` carries a
  nullable `category` and a generic `unit_price REAL`. `line_items` and
  `estimate_line_items` reference it by **UUID TEXT**
  (`price_list_version_id` + `item_id`), not by FK.
- Prices are pinned per line via `price_list_version_id` + `item_id` (tenant
  price-list UUIDs) plus the snapshotted `code`/`unit_price`, so an existing invoice
  is never re-priced when a newer price-list version loads.
- Agent has **no persistent tables** (Smarts are one-shot). The `notes` table and
  all `agent_*` chat tables were dropped (migrations `00005`, `00007`).

## Tables not shown

Auth/infra and supporting tables omitted from the diagram for clarity:
`invites`, `sessions`, `business_profile`, `custom_items`, `tax_rates`,
`recurring_templates` (shown), `audit_log`.
