# SQLite → PostgreSQL Migration with Drizzle ORM

## Context

The app currently uses SQLite via better-sqlite3 with synchronous, raw SQL queries. We're fully replacing SQLite with PostgreSQL using Drizzle ORM for type-safe queries and drizzle-kit for migrations. This affects the entire database layer: connection, schema, migrations, all 14 query modules, repository implementations, 60 route files, and 73 test files.

## Key Decisions

- **Fully replace** SQLite — no dual-driver support
- **Drizzle ORM** with `pg` (node-postgres) driver
- **drizzle-kit** for migrations (replaces custom migrate.ts)
- **PostgreSQL service** added to docker-compose.yml

## Critical Architectural Change: Sync → Async

`better-sqlite3` is synchronous; `pg` is asynchronous. Every query function and repository method that touches the DB must become `async`/`Promise`-returning. TypeScript will catch every missed `await`.

## Steps

### 1. Update dependencies (`apps/app/package.json`)

- **Remove**: `better-sqlite3`, `@types/better-sqlite3`
- **Add deps**: `drizzle-orm`, `pg`
- **Add devDeps**: `drizzle-kit`, `@types/pg`

### 2. Create Drizzle schema (`apps/app/src/lib/db/drizzle-schema.ts`)

Define all 16 tables using Drizzle's `pgTable()`. Key type mappings:

| SQLite | Drizzle/PostgreSQL |
|--------|-------------------|
| `INTEGER PRIMARY KEY AUTOINCREMENT` | `serial('id').primaryKey()` |
| `TEXT` | `text(...)` |
| `REAL` | `doublePrecision(...)` |
| `INTEGER` (booleans: `is_default`, `is_active`, `is_streaming`) | `boolean(...)` |
| `TEXT DEFAULT (datetime('now'))` | `timestamp('...', { withTimezone: true }).defaultNow()` |
| `TEXT DEFAULT (lower(hex(randomblob(16))))` | `uuid('...').defaultRandom()` |
| `CHECK (frequency IN (...))` | `pgEnum` or text + check |

Define all indexes and relations. Delete `apps/app/src/lib/db/schema.ts`.

### 3. Create drizzle-kit config (`apps/app/drizzle.config.ts`)

```ts
export default defineConfig({
  schema: './src/lib/db/drizzle-schema.ts',
  out: './drizzle',
  dialect: 'postgresql',
  dbCredentials: { url: process.env.DATABASE_URL! }
});
```

Generate initial migration: `npx drizzle-kit generate`

### 4. Rewrite connection layer (`apps/app/src/lib/db/connection.ts`)

Replace entire file:
- Create `pg.Pool` from `DATABASE_URL` env var
- Export `db` (Drizzle instance) and `getPool()` (for health check)
- Use Drizzle's programmatic migrator to run migrations on startup
- Remove all `getDb()`, `execute()`, `query()`, `runRaw()`, `save()` exports
- Remove WAL/foreign key PRAGMAs (PostgreSQL handles these natively)

Delete `apps/app/src/lib/db/migrate.ts` (311 lines).

### 5. Make repository interfaces async (`apps/app/src/lib/repositories/interfaces/`)

All 14 interface files — change synchronous read methods to return `Promise<T>`:
- `getInvoices(...)` → `Promise<PaginatedResult<Invoice>>`
- `getInvoice(id)` → `Promise<Invoice | null>`
- `getInvoiceLineItems(id)` → `Promise<LineItem[]>`
- etc. for all interfaces

Files: `InvoiceRepository.ts`, `EstimateRepository.ts`, `ClientRepository.ts`, `PayerRepository.ts`, `CatalogRepository.ts`, `RateTierRepository.ts`, `TaxRateRepository.ts`, `PaymentRepository.ts`, `BusinessProfileRepository.ts`, `DashboardRepository.ts`, `AuditRepository.ts`, `ColumnMappingsRepository.ts`, `RecurringTemplateRepository.ts`, `AiChatRepository.ts`

### 6. Rewrite all 14 query modules (`apps/app/src/lib/db/queries/`)

Convert raw SQL to Drizzle query builder. All functions become `async`. Key SQL translations:

| SQLite | Drizzle/PostgreSQL |
|--------|-------------------|
| `datetime('now')` | `new Date()` or `sql\`now()\`` |
| `last_insert_rowid()` | `.returning({ id: table.id })` |
| `julianday('now') - julianday(col)` | `sql\`EXTRACT(DAY FROM now() - ${col})\`` |
| `strftime('%Y-%m', date)` | `sql\`to_char(${col}, 'YYYY-MM')\`` |
| `date('now', '-11 months')` | `sql\`now() - interval '11 months'\`` |
| `GLOB 'INV-[0-9]*'` | `sql\`${col} ~ '^INV-[0-9]+'\`` |
| `INSERT OR REPLACE` | `.onConflictDoUpdate({ target: [...], set: {...} })` |
| `SUBSTR(col, 5)` | `sql\`substring(${col} from 5)\`` |
| `BEGIN/COMMIT/ROLLBACK` | `db.transaction(async (tx) => { ... })` |
| Bulk `IN (?,?,?)` | `inArray(column, ids)` |

Files (14 + 2 helpers):
- `invoices.ts` (~315 lines) — largest, has date math, transactions, bulk ops
- `estimates.ts` (~287 lines) — similar to invoices
- `clients.ts` (~120 lines)
- `catalog.ts` (~200 lines) — has `INSERT OR REPLACE`
- `recurring-templates.ts` (~275 lines) — has transactions, JSON parsing
- `dashboard.ts` (~126 lines) — has `strftime`, `date()` modifiers
- `payers.ts`, `payments.ts`, `tax-rates.ts`, `rate-tiers.ts`, `business-profile.ts`, `column-mappings.ts`, `ai-chat.ts`, `audit.ts`
- `number-generators.ts` (~27 lines) — has `GLOB`, `SUBSTR`
- `audit.ts` helper (~50 lines) — `logAudit()` function

### 7. Rename and update repository implementations

Rename `apps/app/src/lib/repositories/sqlite/` → `apps/app/src/lib/repositories/postgres/`

- Rename all `Sqlite*Repository.ts` → `Pg*Repository.ts` (14 files)
- Rename `SqliteTransactionFactory.ts` → `PgTransactionFactory.ts` — use `db.transaction()` instead of raw `BEGIN/COMMIT/ROLLBACK`
- Update `index.ts` to use new class names
- Add `await` to all query module calls in repository methods

### 8. Update all 60 route imports

All 60 files that import `from '$lib/repositories/sqlite/index.js'`:
- Change import path to `'$lib/repositories/postgres/index.js'`
- Add `await` to all synchronous repository method calls (reads that were previously sync)

Also fix 2 routes that directly use `$lib/db/connection.js`:
- `apps/app/src/routes/api/export/estimates/+server.ts` — use Drizzle queries
- `apps/app/src/routes/api/import/catalog/+server.ts` — use Drizzle queries + `db.transaction()`

### 9. Update health check (`apps/app/src/routes/health/+server.ts`)

Replace `getDb().prepare('SELECT 1').get()` with `await getPool().query('SELECT 1')`.

### 10. Fix boolean comparisons in Svelte components

SQLite stores booleans as `INTEGER` (0/1). PostgreSQL + Drizzle uses real booleans. Fix:
- `apps/app/src/routes/(app)/console/recurring/[id]/+page.svelte`: `is_active === 1` → `is_active`
- `apps/app/src/routes/(app)/console/recurring/+page.svelte`: `is_active === 1` → `is_active`
- `apps/app/src/lib/components/invoice/InvoiceForm.svelte`: `is_default === 1` → `is_default`
- `apps/app/src/lib/components/estimate/EstimateForm.svelte`: `is_default === 1` → `is_default`

Also update type definitions if `is_default`/`is_active`/`is_streaming` are typed as `number`.

### 11. Update tests (73 test files)

**Query tests** (14 files in `db/queries/*.test.ts`):
- Currently mock `connection.js` exports (`query`, `execute`, `runRaw`)
- New: mock the Drizzle `db` object or create a test-utils helper
- All expectations must handle `async`/`await`

**Route tests** (~10 files in `routes/**/server.test.ts`):
- Mock `$lib/repositories/postgres/index.js` instead of `sqlite`
- Minimal changes beyond import path

**Migration test** (`migrate.test.ts`):
- Delete — drizzle-kit handles migration testing

### 12. Update Docker

**`docker-compose.yml`**: Add postgres service, rename app service:
```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: tallyo
      POSTGRES_PASSWORD: tallyo
      POSTGRES_DB: tallyo
    volumes:
      - pg-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U tallyo"]
  app:
    build: .
    environment:
      - DATABASE_URL=postgresql://tallyo:tallyo@postgres:5432/tallyo
    depends_on:
      postgres: { condition: service_healthy }
```

**`Dockerfile`**: Remove `DB_PATH`, `/data` volume, SQLite dir creation. Add `DATABASE_URL` placeholder.

### 13. Update CI (`.github/workflows/ci.yml`, `release.yml`)

Add PostgreSQL service container:
```yaml
services:
  postgres:
    image: postgres:16-alpine
    env:
      POSTGRES_USER: test
      POSTGRES_PASSWORD: test
      POSTGRES_DB: test
    ports: ['5432:5432']
    options: --health-cmd pg_isready --health-interval 10s --health-timeout 5s --health-retries 5
```

Set `DATABASE_URL=postgresql://test:test@localhost:5432/test` for test/build steps.

### 14. Update documentation

- **`CLAUDE.md`**: SQLite → PostgreSQL + Drizzle ORM, `DB_PATH` → `DATABASE_URL`, update deploy instructions
- **`apps/app/.env.example`**: Add `DATABASE_URL=postgresql://localhost:5432/tallyo`
- **`apps/app/src/lib/db/CLAUDE.md`**: Describe new Drizzle architecture

### 15. Update vite.config.ts

Remove `__PKG_NAME__` injection (was used for SQLite DB path derivation). Keep `__PKG_VERSION__`.

## Files Summary

| Category | Files | Action |
|----------|-------|--------|
| Dependencies | `apps/app/package.json` | Swap deps |
| Schema | `drizzle-schema.ts` (new), `schema.ts` (delete) | Create/delete |
| Config | `drizzle.config.ts` (new) | Create |
| Connection | `connection.ts`, `migrate.ts` (delete) | Rewrite/delete |
| Queries | 14 modules + 2 helpers in `db/queries/` | Rewrite all |
| Interfaces | 14 files in `repositories/interfaces/` | Add async |
| Implementations | 14 + 2 files in `repositories/sqlite/` | Rename + rewrite |
| Routes | 60 files | Update imports + await |
| Svelte components | 4 files | Boolean fixes |
| Tests | ~73 files | Update mocks + async |
| Docker | `Dockerfile`, `docker-compose.yml` | Rewrite |
| CI | `ci.yml`, `release.yml` | Add PG service |
| Docs | `CLAUDE.md`, `.env.example` | Update |

## Verification

1. `npm install` — deps install cleanly
2. `npx drizzle-kit generate` — migration generates
3. Start local PostgreSQL, run `npm run dev` — app starts, migrations apply
4. `npm run check` — TypeScript passes (catches missing awaits)
5. `npm test` — all tests pass
6. `npm run build` — production build succeeds
7. `docker compose up` — app + postgres start, health check passes
8. Manual test: create invoice, view dashboard, run aging report (exercises date math)
