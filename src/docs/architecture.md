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
│   │   ├── import/         # Import wizard steps
│   │   ├── invoice/        # Invoice management
│   │   ├── layout/         # App shell, navbar
│   │   ├── payer/          # Payer form
│   │   ├── pwa/            # PWA reload prompt
│   │   └── shared/         # Reusable UI (Button, Modal, etc.)
│   ├── csv/                # CSV import/export logic
│   ├── db/                 # Database layer
│   │   └── queries/        # SQL query modules + tests
│   ├── import/             # File import processing
│   ├── types/              # Shared TypeScript types
│   └── utils/              # Formatting, PDF, helpers
├── routes/                 # SvelteKit file-based routing
│   ├── catalog/            # /catalog pages
│   ├── clients/            # /clients pages
│   ├── invoices/           # /invoices pages
│   └── settings/           # /settings page
└── docs/                   # Documentation source (VitePress)
```

## Database Layer

- **Schema** is defined in `src/lib/db/schema.ts` with versioned migrations in `migrate.ts`
- **Queries** are organized per entity in `src/lib/db/queries/` with co-located `.test.ts` files
- **UUIDs** are used as primary keys throughout
- **Audit logging** tracks all data mutations with timestamps and change details

## Build & Deployment

The CI pipeline runs on every push to `main`:

1. **Test** — Runs Vitest test suite
2. **Build** — Builds SvelteKit app and VitePress docs
3. **Deploy** — Publishes to GitHub Pages

The SvelteKit app and documentation site are merged into a single static bundle.
