# db/queries/

SQL query modules. Each file exports functions that run parameterized queries via the DB connection. Tests are co-located as `*.test.ts`.

- `invoices.ts` — CRUD for invoices and line items
- `clients.ts` — CRUD for client records
- `catalog.ts` — CRUD for catalog products
- `payers.ts` — CRUD for payer records
- `business-profile.ts` — Read/write business profile settings
- `dashboard.ts` — Aggregation queries for dashboard stats
- `audit.ts` — Insert and query audit log entries
- `column-mappings.ts` — Saved column mappings for CSV imports
- `rate-tiers.ts` — Tiered pricing rate queries
