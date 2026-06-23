# Tallyo

Self-hosted, source-available (AGPL-3.0) invoice management web app — Go backend (chi + SQLite/modernc + sqlc) serving an embedded SvelteKit SPA.

> **Architecture note.** This was rewritten from an Electron + SvelteKit desktop app to a **single-binary Go web service** serving an embedded SvelteKit SPA. The Go implementation lives at the repo root (`cmd/`, `internal/`, `web/`); the legacy Electron/SvelteKit tree (`src/`, `electron/`, `drizzle/`, root `package.json`) is **superseded** and slated for removal. Design/plan docs: `docs/superpowers/{specs,plans}/`.

## Tech Stack

- **Backend:** Go 1.26 — `cmd/tallyo` `serve` command (single binary). chi v5 router, REST JSON API.
- **Database:** SQLite via **modernc.org/sqlite** (pure-Go, no cgo) + **sqlc** (typed queries) + **goose** (embedded migrations, run on startup). `_txlock=immediate` DSN.
- **Auth:** email/password (bcrypt), server-side cookie sessions via `alexedwards/scs/v2` (SQLite-backed). Single-org, multi-user; first-run setup + manual invite links.
- **Realtime:** Server-Sent Events (`/api/events`) — an in-process hub broadcasts entity-change events; the SPA refetches into Svelte runes.
- **Frontend:** SvelteKit + `@sveltejs/adapter-static` (SPA, `200.html` fallback), Svelte 5 runes, Tailwind CSS 4, built and embedded via `//go:embed`.
- **PDF:** `johnfercher/maroto/v2` (pure-Go). **Import/Export:** stdlib `encoding/csv` + `xuri/excelize/v2` (pure-Go).
- **Testing:** Go stdlib `testing`; `svelte-check` + Vitest for the frontend.
- **License:** AGPL-3.0 (verbatim, copyleft) — `LICENSE`. The binary is **cgo-free** (`CGO_ENABLED=0 go build ./cmd/tallyo`).

## Project Layout

The backend is a **modular monolith of vertical domain slices**. Each domain owns
its `repository.go` + `service.go` + `handler.go` + types in one package; the
handler self-registers its routes via `Routes(r chi.Router)`. Slices depend on the
shared platform packages, never on each other directly — cross-domain reads go
through the central `db/gen`; cross-domain writes/behaviour go through small
interfaces declared by the consumer and wired in `internal/app`.

- `cmd/tallyo/main.go` (~40 lines) — parses flags, then calls `app.Run`.
- `internal/app/` — composition root: resolve data dir → open DB → migrate → build
  every slice's service+handler → assemble the chi router (`server.go`: middleware,
  `/api` group, role gates, SPA catch-all) → graceful shutdown; owns the per-tenant
  overdue+recurring sweeps (`sweep.go`, launch + hourly ticker).
  Also holds the auth/invite/signup HTTP handlers (kept here to avoid an
  `auth → httpx → auth` cycle).
- **Platform (cross-cutting, shared by slices):**
  - `internal/db/` — the single modernc `*sql.DB` connection (`sqlite.go`),
    `migrate.go` (goose), `migrations/*.sql`, `queries/*.sql` (sqlc source),
    `gen/` (sqlc output, ONE central package — do not edit, do not split). Both
    control and tenant repos take this one shared `db.Executor`.
  - `internal/audit/` — `WithTx` audited-mutation wrapper + `Log`/`Changes`.
  - `internal/numbering/` — concurrency-safe document numbers (tx-scoped + retry).
  - `internal/reqctx/` — tenant/user request context.
  - `internal/realtime/` — SSE hub + the `/api/events` stream handler.
  - `internal/httpx/` — domain-agnostic HTTP helpers: `WriteJSON`/`WriteError`/
    `WriteValidationError`/`DecodeJSON`/`ParseID`, middleware (`Recover`,
    `RequestLogger`, `RequireAuth`, `RequireRole`, `RequirePlatformAdmin`), logging,
    `SPAHandler`.
  - `internal/pdf/` (maroto render), `internal/importer/` (generic price-list
    parse/map: `ApplyMapping` turns header→field column mappings into typed rows).
- `internal/billing/` — the shared **billing-document core**: `LineItem(Input)`
  types, `ComputeTotals`/`Round2`, `SnapshotBuilder` (reads gen), and the
  `LineValidator`. Catalogue lines price from `items.unit_price`; the validator
  applies tax (per-line `taxable` × the tenant default rate) and non-negativity —
  there are no pricing zones, price caps, or plan windows. The
  invoice/estimate/recurring slices compose it.
- **Domain slices:** `internal/{invoice,estimate,recurring,session,client,
  payer,taxrate,businessprofile,customitem,pricelist,auth,agent,export}`.
  `invoice` includes payment. `invoice` declares `SessionLinker`; `session`
  declares `InvoiceChecker` — these break the invoice↔session cycle. `agent` is a consumer
  slice exposing one-shot **Smarts** (gather → propose → apply via a forced
  single-tool LLM call, then deterministic apply); its tools take interfaces and
  it has no persistent agent tables.
- `web/` — SvelteKit SPA (`src/lib/api`, `src/lib/stores`, `src/routes`); `web/embed.go` embeds `web/build`.

## Run

```bash
# Build the SPA first (the Go build embeds web/build):
cd web && npm install && npm run build && cd ..
# Run the server (single binary):
go run ./cmd/tallyo --port 8080  # or: go build -o bin/tallyo ./cmd/tallyo && ./bin/tallyo
# Frontend dev with hot reload (Vite proxies /api → :8080):
cd web && npm run dev
```
Flags: `--port`, `--data-dir` (else `DATA_DIR` env, else `./data`), `--secure-cookie` (behind TLS). DB file: `<data-dir>/tallyo.db`.

## Commands

- `go test ./...` — Go tests (add `-race` for the full gate).
- `go vet ./...` ; `gofmt -l .` — must be clean.
- `CGO_ENABLED=0 go build ./cmd/tallyo` — verify the cgo-free single binary.
- `"$(go env GOPATH)/bin/sqlc" generate` — regenerate `internal/db/gen` from `queries/*.sql`.
- `cd web && npm run check` — svelte-check (0 errors / 0 warnings) ; `npm run build` — emit `web/build`.

## Conventions

- sqlc source SQL in `internal/db/queries/`; never hand-edit `internal/db/gen/`.
- Each domain is a **vertical slice** (`internal/<domain>/`): handler → service → repository → sqlc gen, all in one package. Within a slice, handlers call its service, the service calls its repository, the repository calls gen — never skip a layer.
- **No slice imports another slice.** Cross-domain reads use the central `db/gen` (enrichment joins live in SQL); cross-domain writes/behaviour use a small interface declared by the consumer slice and wired in `internal/app` (e.g. `invoice.SessionLinker`, `session.InvoiceChecker`). The invoice/estimate/recurring slices share `internal/billing` (line items, totals, snapshots, validator).
- Every DB mutation is audited (via `audit.WithTx`) and broadcasts an SSE event from the service after commit.
- JSON is camelCase (Go struct json tags); list endpoints return `[]` (non-nil) when empty.
- **UUID addressing.** The HTTP/JSON API addresses every entity by its **uuid** — paths are `/{...UUID}` (e.g. `/invoices/{invoiceUUID}`) and every JSON `id` / `*Id` field is a uuid string. The int64 PK is internal-only: never in a URL or JSON payload. Handlers resolve `uuid → row` at the boundary (`httpx.ParseUUID` + a `GetXByUUID` lookup) and operate on the int PK internally; inbound FK uuids resolve to int before insert. SvelteKit routes use `[uuid]` params.
- Clean-break data model (fresh goose schema; no migration from the old Electron `tallyo.db`).
- Commits follow Conventional Commits.

## Database

- **ERD / data-model map: [`docs/data-model.md`](docs/data-model.md)** — Mermaid diagram of tables + relationships. Keep it in sync when a migration changes the schema.
- SQLite (modernc.org/sqlite, pure-Go) + sqlc + goose. WAL, `foreign_keys=ON`, `busy_timeout=5000`, `_txlock=immediate` (all mutations take the write lock at BEGIN).
- **Single SQLite instance, logical tenancy.** All tables — global/reference (tenants, users, invites, sessions, global audit_log) and every tenant's business data, including the price list — live in one file (`<data-dir>/tallyo.db`). The app opens one shared `*sql.DB` (the `db.Executor` interface) and passes it to both the control repos and the tenant services. Tenancy is **logical only**: every business row carries a `tenant_id` column and every query guards `WHERE tenant_id = ?`; `reqctx` carries the request's tenant for those guards + audit. `tenant_id` and author user ids are NOT foreign keys — validated in app. (The model was simplified from an earlier DB-per-tenant design; that historical spec lives under `docs/superpowers/specs/`.)
- Migrations are embedded in **two goose sequences** (distinct version tables): `internal/db/migrations/control/*.sql` and `internal/db/migrations/tenant/*.sql`. `appdb.Migrate(db)` runs both sequences into the one file at startup. sqlc reads the control dir + the tenant business-table file (the tenant `audit_log` migration is excluded to avoid a duplicate-table; goose still applies it). Add a migration to the right dir then `sqlc generate`.
- `DATA_DIR` / `--data-dir` override the data dir (default `./data`).

### Price list (versioned, tenant-owned)

- The catalogue is a generic, **tenant-owned price list** — the two tables
  (`price_list_versions`, `items`) live in the single DB under the tenant goose
  sequence (`internal/db/migrations/tenant/`), scoped per tenant by `tenant_id`,
  and every tenant populates its own. An
  `items` row carries `code`, `name`, `unit`, a nullable `category`, a generic
  `unit_price` (base per-unit price), and `taxable`. Each release is its own
  `price_list_versions` row; loading a newer one never mutates prior versions, and
  prices are pinned per invoice line (`price_list_version_id` + `item_id` on
  `line_items`, stored as tenant price-list UUIDs) so existing invoices are never
  re-priced. There are no pricing zones, price caps, plan windows, or client types.
- Read endpoints (`GET …/price-list/versions`, `…/versions/{versionUUID}/items`)
  serve the tenant-scoped tables.
- **Generic upload-and-map import.** Ingest is a two-step, file-format-agnostic
  flow (CSV/XLSX), both gated by **owner/admin**: `POST …/price-list/import/inspect`
  returns the detected headers + a row sample; the SPA mapping wizard maps each
  source header to a target field; `POST …/price-list/import/commit` applies it.
  The pipeline is `importer.ApplyMapping` (header→field → typed rows) feeding the
  `pricelist` slice's `Inspect` / `ImportMapped`. Each item gets a single
  `unit_price`.

## Coding Rules (NASA Power of 10, adapted)

Apply to all new and modified code. Adapted from JPL's "Power of Ten" for safety-critical C; reinterpreted for **Go** (backend) and TypeScript/Svelte (frontend). For Go: `return fmt.Errorf(...)` instead of `throw`; `go vet`/`staticcheck`/`gofmt` clean instead of the TS/svelte-check/ESLint gate in rule 10; check every error return (no `_`-discards on fallible calls) for rule 7.

1. **Simple control flow.** No `goto`, no recursion (unless provably bounded and justified in a comment). Prefer flat early-return over nested branching.
2. **Bounded loops.** Every loop must have a statically obvious upper bound. No `while (true)` without an explicit break condition tied to a bounded counter or external signal.
3. **No dynamic allocation after init.** In hot paths, avoid allocating new objects/arrays per iteration. Pre-size arrays, reuse buffers, and prefer iteration over `.map().filter().reduce()` chains when shape is fixed.
4. **Short functions.** Aim for ≤ 60 lines per function (one screen). Split when a function does more than one thing.
5. **Assertion density.** At least two runtime checks per non-trivial function — validate inputs at module boundaries (HTTP, DB, file I/O, user input). Use `throw new Error(...)` for invariant violations; never silently coerce.
6. **Smallest scope for data.** Declare variables at innermost scope. No module-level mutable state unless it represents a singleton resource (DB connection, i18n store). Prefer `const`; use `let` only when reassigned.
7. **Check every return value.** No ignored Promises, no swallowed errors. Every `await` is either inside a `try/catch` or its rejection is a documented programmer error. No bare `catch {}` — log or rethrow.
8. **Limit preprocessor / metaprogramming.** Avoid `eval`, `Function()`, dynamic `import()` of computed paths, and clever type gymnastics. Prefer explicit code over generated code.
9. **Restrict pointer/reference indirection.** Limit object-graph traversal to one level of optional chaining per expression. Destructure once at function entry rather than reaching deep into arguments throughout the body.
10. **Compile clean at max strictness.** Zero TypeScript errors, zero `svelte-check` warnings, zero ESLint warnings on every commit. `// @ts-ignore`, `// svelte-ignore`, and `any` require an inline comment explaining why.
