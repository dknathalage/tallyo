# Architecture

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Framework | SvelteKit with Svelte 5 |
| Language | TypeScript (strict mode) |
| Styling | Tailwind CSS 4 |
| Database | SQL.js (SQLite compiled to WebAssembly) |
| Persistence | IndexedDB |
| PDF | jsPDF + jspdf-autotable |
| CSV | PapaParse |
| Excel | SheetJS (xlsx) |
| PWA | Workbox via @vite-pwa/sveltekit |
| Build | Vite |
| Testing | Vitest |
| Deploy | GitHub Pages via GitHub Actions |

## Local-First Design

The app has no backend server. All data processing happens in the browser:

1. **SQL.js** loads a WebAssembly build of SQLite, providing a full relational database in the browser
2. **IndexedDB** persists the database file between sessions
3. The **service worker** (Workbox) caches app assets for offline use

This means the app works without an internet connection after the first visit.

## Project Structure

```
src/
├── lib/                    # Shared library code ($lib/)
│   ├── components/         # Svelte components by domain
│   │   ├── catalog/        # Catalog management
│   │   ├── client/         # Client management
│   │   ├── csv/            # Import/export UI
│   │   ├── estimate/       # Estimate management
│   │   ├── import/         # Import wizard steps
│   │   ├── invoice/        # Invoice management
│   │   ├── layout/         # App shell, navbar
│   │   ├── payer/          # Payer / Bill-To form
│   │   ├── pwa/            # PWA reload prompt
│   │   └── shared/         # Reusable UI (Button, Modal, etc.)
│   ├── csv/                # CSV import/export logic
│   ├── db/                 # Database layer
│   │   └── queries/        # SQL query modules + tests
│   ├── i18n/               # Translations and locale types
│   ├── import/             # File import processing
│   ├── stores/             # Svelte stores (theme, i18n, announcer)
│   ├── types/              # Shared TypeScript types
│   └── utils/              # Formatting, PDF, currency helpers
├── routes/                 # SvelteKit file-based routing
│   ├── catalog/            # /catalog pages
│   ├── clients/            # /clients pages
│   ├── estimates/          # /estimates pages
│   ├── invoices/           # /invoices pages
│   └── settings/           # /settings page
└── docs/                   # Documentation source (VitePress)
```

## Database Layer

- **Schema** is defined in `src/lib/db/schema.ts` with versioned migrations in `migrate.ts`
- **Queries** are organized per entity in `src/lib/db/queries/` with co-located `.test.ts` files
- **UUIDs** are used as primary keys throughout
- **Audit logging** tracks all data mutations with timestamps and change details

## Database Tables

| Table | Purpose |
|-------|---------|
| `clients` | Client contact records |
| `invoices` | Invoice header data (number, dates, totals, status, currency, snapshots) |
| `line_items` | Invoice line items (linked to invoices via cascade) |
| `estimates` | Estimate header data (same structure as invoices plus `valid_until` and `converted_invoice_id`) |
| `estimate_line_items` | Estimate line items (linked to estimates via cascade) |
| `catalog_items` | Product and service catalog |
| `rate_tiers` | Named pricing tiers |
| `catalog_item_rates` | Per-tier rates for catalog items |
| `payers` | Bill-to party records |
| `business_profile` | Singleton business settings (name, logo, default currency) |
| `column_mappings` | Saved CSV/Excel import column mappings |
| `audit_log` | Change history for all entities |

All tables use auto-increment integer primary keys with a secondary `uuid` column for external references.

## Build & Deployment

The CI pipeline runs on every push to `main`:

1. **Test** — Runs Vitest test suite
2. **Build** — Builds SvelteKit app and VitePress docs
3. **Deploy** — Publishes to GitHub Pages

The SvelteKit app and documentation site are merged into a single static bundle.

## Pre-Commit Hooks

The repository uses Husky to run checks before every commit:

1. **Vitest** — Runs the full test suite
2. **docs:build** — Builds the VitePress documentation site

If either step fails, the commit is rejected. This ensures that tests pass and documentation stays buildable at every commit.
