# Tallyo

Self-hosted, open-source invoice management app distributed as a global npm CLI. SQLite via better-sqlite3 + Drizzle ORM.

## Tech Stack

- **Framework:** SvelteKit with Svelte 5, TypeScript (strict), Node adapter
- **Styling:** Tailwind CSS 4 via Vite plugin
- **Database:** SQLite via better-sqlite3 + Drizzle ORM
- **PDF:** jsPDF + autotable
- **Import/Export:** PapaParse (CSV), XLSX (Excel)
- **Testing:** Vitest
- **Distribution:** Global npm CLI (`tallyo`) via `bin/tallyo.js`

## Project Layout

- `bin/tallyo.js` — CLI entry: resolves data dir, picks port, runs migrations, boots SvelteKit Node server, opens browser
- `src/lib/` — Shared library: components, database, utilities
- `src/lib/db/` — Database connection, schema, migrations, query modules
- `src/lib/repositories/` — Data access layer (interfaces + SQLite implementations)
- `src/lib/utils/` — Helpers (currency, formatting, PDF)
- `src/routes/` — SvelteKit pages and server load functions
- `drizzle/` — Generated Drizzle migrations (shipped in published tarball)
- `build/` — SvelteKit Node adapter output (built before publish)

## Run

End users:

```bash
npm install -g tallyo
tallyo
```

Local dev:

```bash
npm run dev          # Vite dev server at http://localhost:5173
npm run build        # Production build into build/
npm link             # Use `tallyo` globally from working tree
```

## Commands

- `npm run dev` — Start dev server (http://localhost:5173)
- `npm run build` — Production build
- `npm test` — Run Vitest tests
- `npm run test:coverage` — Run tests with coverage report
- `npm run check` — TypeScript check
- `npx drizzle-kit generate` — Generate Drizzle migrations
- `npx drizzle-kit migrate` — Run Drizzle migrations

## Conventions

- Database queries live in `src/lib/db/queries/` with co-located `.test.ts` files
- Use repositories (`$lib/repositories`) in routes — never `$lib/db/queries` directly
- Components are grouped by domain under `src/lib/components/`
- All database mutations are audit-logged
- Commits follow Conventional Commits

## Database

- SQLite (better-sqlite3) + Drizzle ORM
- Migrations managed by Drizzle Kit (`npx drizzle-kit generate` / `npx drizzle-kit migrate`)
- Database file stored in `DATA_DIR` (default: `~/.tallyo/tallyo.db`); `bin/tallyo.js` runs migrations on startup
