# Invoice Manager

Local-first invoice management PWA. All data stays on the user's device via IndexedDB + SQL.js (in-browser SQL). No backend server.

## Tech Stack

- **Framework:** SvelteKit with Svelte 5, TypeScript (strict)
- **Styling:** Tailwind CSS 4 via Vite plugin
- **Database:** SQL.js (WebAssembly SQLite) with IndexedDB persistence
- **PDF:** jsPDF + autotable
- **Import/Export:** PapaParse (CSV), XLSX (Excel)
- **PWA:** Workbox service worker, installable manifest
- **Testing:** Vitest
- **Deploy:** GitHub Pages via GitHub Actions

## Project Layout

- `src/lib/` — Shared library: components, database, utilities
- `src/routes/` — SvelteKit pages (dashboard, invoices, clients, catalog, settings)
- `static/` — Static assets (icons, WASM binary, favicon)
- `build/` — Production build output (generated)
- `.github/workflows/` — CI/CD pipeline

## Commands

- `npm run dev` — Start dev server
- `npm run build` — Production build
- `npm run test` — Run Vitest tests
- `npm run preview` — Preview production build

## Conventions

- Database queries live in `src/lib/db/queries/` with co-located `.test.ts` files
- Components are grouped by domain under `src/lib/components/`
- Routes follow SvelteKit file-based routing with `+page.svelte` files
- UUIDs are used as primary keys throughout
- All database mutations are audit-logged
- Update user documentation (`src/docs/`) when a feature change impacts user-facing functionality
