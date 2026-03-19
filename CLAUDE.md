# Invoice Manager

Self-hosted, open-source invoice management app. Turborepo monorepo with PostgreSQL via pg + Drizzle ORM.

## Tech Stack

- **Monorepo:** Turborepo with npm workspaces
- **Framework:** SvelteKit with Svelte 5, TypeScript (strict)
- **Styling:** Tailwind CSS 4 via Vite plugin
- **Database:** PostgreSQL via pg + Drizzle ORM
- **PDF:** jsPDF + autotable
- **Import/Export:** PapaParse (CSV), XLSX (Excel)
- **Testing:** Vitest
- **Deploy:** Node.js server (`node build/index.js`)

## Project Layout

- `apps/app/` — SvelteKit invoice app
- `apps/app/src/lib/` — Shared library: components, database, utilities
- `apps/app/src/lib/db/` — Database connection, schema, migrations, query modules
- `apps/app/src/lib/repositories/` — Data access layer (interfaces + SQLite implementations)
- `apps/app/src/lib/utils/` — Helpers (currency, formatting, PDF)
- `apps/app/src/routes/` — SvelteKit pages and server load functions

## Commands

- `npm run dev` — Start dev server (http://localhost:5173)
- `npm run build` — Production build
- `npm test` — Run Vitest tests (224 tests)
- `npm run test:coverage` — Run tests with coverage report
- `npm run check` — TypeScript check
- `npx drizzle-kit generate` — Generate Drizzle migrations
- `npx drizzle-kit migrate` — Run Drizzle migrations

All root commands run via Turborepo. You can also run app-specific commands:
- `npm run --workspace=@tallyo/app dev`

## Production Deployment

Ensure PostgreSQL is running and accessible before starting the app.

```bash
npm run build
DATABASE_URL=postgresql://user:pass@localhost:5432/tallyo PORT=3002 HOST=0.0.0.0 node apps/app/build/index.js
```

Or use Docker Compose:

```bash
docker compose up -d
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3000` | HTTP port |
| `HOST` | `0.0.0.0` | Bind address |
| `NODE_ENV` | `development` | Environment |
| `DATABASE_URL` | `postgresql://localhost:5432/tallyo` | PostgreSQL connection URL |

### Health Check

```bash
curl http://localhost:3002/health
# {"status":"ok","db":"connected"}
```

## Conventions

- Database queries live in `apps/app/src/lib/db/queries/` with co-located `.test.ts` files
- Use repositories (`$lib/repositories`) in routes — never `$lib/db/queries` directly
- Components are grouped by domain under `apps/app/src/lib/components/`
- All database mutations are audit-logged
- Commits follow Conventional Commits (enforced by commitlint)

## Database

- PostgreSQL via pg + Drizzle ORM
- Migrations managed by Drizzle Kit (`npx drizzle-kit generate` / `npx drizzle-kit migrate`)
- Connection URL: `DATABASE_URL` env var (default: `postgresql://localhost:5432/tallyo`)
