# Invoice Manager

Self-hosted, open-source invoice management app. Server-side SQLite via better-sqlite3.

## Tech Stack

- **Framework:** SvelteKit with Svelte 5, TypeScript (strict)
- **Styling:** Tailwind CSS 4 via Vite plugin
- **Database:** SQLite via better-sqlite3 (server-side)
- **PDF:** jsPDF + autotable
- **Import/Export:** PapaParse (CSV), XLSX (Excel)
- **Testing:** Vitest
- **Deploy:** Node.js server (`node build/index.js`)

## Project Layout

- `src/lib/` — Shared library: components, database, utilities
- `src/lib/db/` — Database connection, schema, migrations, query modules
- `src/lib/repositories/` — Data access layer (interfaces + SQLite implementations)
- `src/lib/utils/` — Helpers (currency, formatting, PDF)
- `src/routes/` — SvelteKit pages and server load functions

## Commands

- `npm run dev` — Start dev server (http://localhost:5173)
- `npm run build` — Production build
- `npm test` — Run Vitest tests (224 tests)
- `npm run test:coverage` — Run tests with coverage report
- `npm run check` — TypeScript check

## Production Deployment

```bash
npm run build
PORT=3002 HOST=0.0.0.0 node build/index.js
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3000` | HTTP port |
| `HOST` | `0.0.0.0` | Bind address |
| `NODE_ENV` | `development` | Environment |
| `DB_PATH` | `~/.invoices/invoices.db` | SQLite database path |

### Health Check

```bash
curl http://localhost:3002/health
# {"status":"ok","db":"connected"}
```

## Conventions

- Database queries live in `src/lib/db/queries/` with co-located `.test.ts` files
- Use repositories (`$lib/repositories`) in routes — never `$lib/db/queries` directly
- Components are grouped by domain under `src/lib/components/`
- All database mutations are audit-logged
- Commits follow Conventional Commits (enforced by commitlint)

## Database

- SQLite via better-sqlite3 (server-side, synchronous writes)
- Migrations run on startup via `src/lib/db/migrate.ts`
- DB location: `~/.invoices/invoices.db` (or `DB_PATH` env var)
