# Invoice Manager

Local-first invoice management PWA. All data stays on the user's device via IndexedDB + SQL.js (in-browser SQL). No backend server.

## Tech Stack

- **Framework:** SvelteKit with Svelte 5, TypeScript (strict)
- **Styling:** Tailwind CSS 4 via Vite plugin + `@tailwindcss/typography` for prose
- **Markdown:** mdsvex (`.md` files as Svelte components)
- **Database:** SQL.js (WebAssembly SQLite) with IndexedDB persistence
- **PDF:** jsPDF + autotable
- **Import/Export:** PapaParse (CSV), XLSX (Excel)
- **PWA:** Workbox service worker, installable manifest
- **Testing:** Vitest
- **Deploy:** GitHub Pages via GitHub Actions

## Project Layout

- `src/lib/` — Shared library: components, database, utilities
- `src/routes/(app)/` — App pages (dashboard, invoices, clients, catalog, settings) — wrapped in `FileGate > AppShell`
- `src/routes/(docs)/docs/` — Documentation pages (mdsvex markdown) — standalone layout, no database dependency
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
- Routes follow SvelteKit file-based routing with `+page.svelte` and `+page.md` files
- Route groups `(app)` and `(docs)` separate DB-dependent app from standalone docs
- UUIDs are used as primary keys throughout
- All database mutations are audit-logged
- Update user documentation (`src/routes/(docs)/docs/`) when a feature change impacts user-facing functionality
