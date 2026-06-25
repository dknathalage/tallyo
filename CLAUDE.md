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
  - `internal/ids/` — `New()` mints a UUIDv7 (time-ordered) string id; the sole id source for every row's PK and FK.
  - `internal/db/` — the single modernc `*sql.DB` connection (`sqlite.go`),
    `migrate.go` (goose), `migrations/*.sql`, `queries/*.sql` (sqlc source),
    `gen/` (sqlc output, ONE central package — do not edit, do not split). Both
    control and tenant repos take this one shared `db.Executor`.
  - `internal/audit/` — `WithTx` audited-mutation wrapper + `Log`/`Changes`.
  - `internal/numbering/` — concurrency-safe document numbers (tx-scoped + retry).
  - `internal/reqctx/` — tenant/user request context.
  - `internal/realtime/` — SSE hub + the `/api/events` stream handler.
  - `internal/httpx/` — domain-agnostic HTTP helpers: `WriteJSON`/`WriteError`/
    `WriteValidationError`/`WriteServiceError`/`DecodeJSON`/`ParseUUID`, middleware
    (`Recover`, `RequestLogger`, `RequireAuth`, `RequireRole`,
    `RequirePlatformAdmin`), logging, `SPAHandler`. `WriteServiceError(w, err)`
    maps a service's typed/sentinel error to HTTP (404/409/422/500) and returns
    whether it wrote — the single error-mapping home for every handler.
  - `internal/apperr/` — shared outcome sentinels (`ErrNotFound`, `ErrConflict`)
    services return and `httpx.WriteServiceError` maps, plus the `Validation`
    interface and a stdlib-only `ValidationError`/`FieldError` the simple CRUD
    slices use for cheap field checks (they can't import `billing` —
    `billing`'s tests import `taxrate`/`client`, so that would cycle). Kept
    separate so `httpx` need not import `billing`.
  - `internal/events/` — `Notifier{hub, entity}` with `Created/Updated/Deleted(ctx,
    id)`; one per service, replaces inline `realtime.Event{...}` literals.
  - `internal/pdf/` (maroto render), `internal/importer/` (generic price-list
    parse/map: `ApplyMapping` turns header→field column mappings into typed rows).
- `internal/billing/` — the shared **billing-document core**: `LineItem(Input)`
  types, `ComputeTotals`/`Round2`, `SnapshotBuilder` (reads gen), and the
  `LineValidator`. A catalogue line carries a `catalogueItemId`; the validator
  reads that exact `catalogue_items` row (no version-by-date, no code lookup),
  prices from its `unit_price`, snapshots code/name/taxable, then applies tax
  (per-line `taxable` × the tenant default rate) and non-negativity — there are
  no pricing zones, price caps, or plan windows. The invoice/estimate/recurring
  slices compose it.
- **Domain slices:** `internal/{invoice,estimate,recurring,session,client,
  payer,taxrate,businessprofile,catalogue,auth,smarts,export}`.
  `invoice` includes payment. `invoice` declares `SessionLinker`; `session`
  declares `InvoiceChecker` — these break the invoice↔session cycle. `smarts` is the
  curated AI layer: a small set of user-initiated, button-triggered **Smarts**, each
  `gather → propose → apply` returning an editable draft — no agent loop, no chat, no
  persisted conversation/step tables. A thin Anthropic SDK wrapper (`llm.go`) exposes
  `Propose` (one forced-single-tool call) and `ProposeGrounded` (a bounded read-tool
  loop where the model uses a tenant-scoped, all-fields catalogue `search` to ground
  specifics, then emits a final commit). Four Smarts exist: draft-invoice-from-sessions
  (grounded; creates a draft invoice the user lands in), suggest-line-items,
  draft-overdue-follow-up, map-price-list-import. The model proposes structure;
  deterministic code prices from the catalogue and the invoice service validates. Its
  tools take interfaces; routes return 503 when no `ANTHROPIC_API_KEY`.
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
- **Slice anatomy — every slice has the same shape (learn one, know them all).** Each
  domain is a **vertical slice** (`internal/<domain>/`), **one flat Go package**,
  organized by file (never by layer-subpackage — that forces export-everything and
  invites import cycles). The canonical files + layer contract:
  - `handler.go` — **HTTP only**: `DecodeJSON` → call service → `httpx.WriteServiceError(w, err)` → `WriteJSON`. No field validation here.
  - `service.go` — **the brain**: validates input (`in.Validate()`), reads tenant from `reqctx`, orchestrates the repo, broadcasts via `events.Notifier`. Returns typed/sentinel errors (`apperr.ErrNotFound`, `*billing.ValidationError`, slice-local conflict sentinels) — never `(nil, nil)` for not-found.
  - `repository.go` — **thin**: `audit.WithTx` + `gen` call + row→domain map; trusts its input (no re-validation); translates `sql.ErrNoRows` → `apperr.ErrNotFound`.
  - `query.go` (optional, list/filter read SQL) + `types.go` (domain struct + `Input` struct + `Input.Validate() error`).
  Within a slice never skip a layer (handler → service → repository → gen). **All CRUD/billing slices conform to this shape (the conformance pass: `docs/superpowers/specs/2026-06-25-slice-consistency-design.md`); new slices copy it. `smarts` is the AI-orchestration exception — it composes other slices.**
- **Flat layout, predictable names.** Keep `internal/<slice>/` flat — no `domain/`/`platform/` grouping folders (longer paths cost navigation, buy nothing). Use the **identical filenames** above in every slice. Split any file over ~400 lines on a predictable seam (`query.go`, `payment_repository.go`) so each file is one focused concept. Symbol names unique + greppable.
- **Validation lives in the service**, once, before the repo — testable without HTTP. **Error mapping lives once** in `httpx.WriteServiceError`; handlers don't hand-roll `errors.Is` chains.
- **No slice imports another slice.** Cross-domain reads use the central `db/gen` (enrichment joins live in SQL); cross-domain writes/behaviour use a small interface declared by the consumer slice and wired in `internal/app` (e.g. `invoice.SessionLinker`, `session.InvoiceChecker`). The invoice/estimate/recurring slices share `internal/billing` — line items, totals, snapshots, validator, **and the shared billing-document mechanics `billing.NextNumber(…, prefix)` + `billing.InsertLineItems(…)`** (these live in `billing`, not the `invoice` slice, so estimate/recurring build their invoices inline via shared `gen` without importing `invoice`).
- Every DB mutation is audited (via `audit.WithTx`) and broadcasts an SSE event from the service after commit — via the slice's `events.Notifier` (`s.events.Created/Updated/Deleted(ctx, id)`), not an inline `realtime.Event{...}` literal.
- JSON is camelCase (Go struct json tags); list endpoints return `[]` (non-nil) when empty.
- **UUIDv7 ids, end to end.** Every row's primary key **is** a UUIDv7 string (`id`) — the same value in the URL, the JSON, and as the DB key. There is no separate int PK and no `uuid → int` resolution step: handlers parse the uuid (`httpx.ParseUUID`) and query by it directly as the PK, and inbound FK uuids are stored as-is. JSON `id` / `*Id` fields are uuid strings; paths are `/{...UUID}` (e.g. `/invoices/{invoiceUUID}`); SvelteKit routes use `[uuid]` params. All ids are minted by `internal/ids.New()` (UUIDv7, time-ordered) — never `uuid.NewString()` (v4).
- Clean-break data model (fresh goose schema; no migration from the old Electron `tallyo.db`).
- **Gotchas: [`docs/gotchas.md`](docs/gotchas.md)** — hard-won traps (e.g. seeding lazily-mounted modal forms with `$effect.pre`). Read before touching a listed area; add an entry when you hit a new one.
- Commits follow Conventional Commits.

## Database

- **ERD / data-model map: [`docs/data-model.md`](docs/data-model.md)** — Mermaid diagram of tables + relationships. Keep it in sync when a migration changes the schema.
- SQLite (modernc.org/sqlite, pure-Go) + sqlc + goose. WAL, `foreign_keys=ON`, `busy_timeout=5000`, `_txlock=immediate` (all mutations take the write lock at BEGIN).
- **Single SQLite instance, logical tenancy.** All tables — global/reference (tenants, users, invites, sessions, global audit_log) and every tenant's business data, including the price list — live in one file (`<data-dir>/tallyo.db`). The app opens one shared `*sql.DB` (the `db.Executor` interface) and passes it to both the control repos and the tenant services. Tenancy is **logical only**: every business row carries a `tenant_id` column and every query guards `WHERE tenant_id = ?`; `reqctx` carries the request's tenant for those guards + audit. `tenant_id` and author user ids are NOT foreign keys — validated in app. (The model was simplified from an earlier DB-per-tenant design; that historical spec lives under `docs/superpowers/specs/`.)
- Migrations are embedded in **two goose sequences** (distinct version tables): `internal/db/migrations/control/*.sql` and `internal/db/migrations/tenant/*.sql`. `appdb.Migrate(db)` runs both sequences into the one file at startup. sqlc reads the control dir + the tenant business-table file (the tenant `audit_log` migration is excluded to avoid a duplicate-table; goose still applies it). Add a migration to the right dir then `sqlc generate`.
- `DATA_DIR` / `--data-dir` override the data dir (default `./data`).

### Catalogue (per-item versioned, tenant-owned)

- The catalogue is a generic, **tenant-owned** list of priced line templates in
  one append-only table, `catalogue_items` (tenant goose sequence,
  `internal/db/migrations/tenant/`). Every row carries `tenant_id` and every
  query guards `WHERE tenant_id = ?`, so each tenant populates its own. A row
  carries `code` (optional, the import upsert key), `name`, `unit`, nullable
  `category`, a generic `unit_price`, `taxable`, plus the versioning columns
  `logical_id` + `version` + `is_current`. The live catalogue is
  `WHERE is_current = 1`.
- **Per-item copy-on-write versioning.** Rows sharing a `logical_id` are the
  version history of one item. Editing an item mutates its current row **in
  place** UNLESS that row is already referenced by an invoice/estimate line, in
  which case it **forks** a new version (the old row stays frozen, still
  referenced). Delete tombstones the `logical_id` (all rows `is_current = 0`).
  Invoice lines pin the exact version via a single `line_items.catalogue_item_id`
  FK, so existing invoices are never re-priced. There are no pricing zones, price
  caps, plan windows, release snapshots, or client types. (Replaced the earlier
  `custom_items` + `price_list_versions`/`items` split; see
  `docs/superpowers/specs/2026-06-25-catalogue-merge-design.md`.)
- CRUD endpoints: `GET/POST …/catalogue`, `GET/PUT/DELETE …/catalogue/{uuid}`,
  `POST …/catalogue/bulk-delete` (serve the current rows).
- **Generic upload-and-map import.** Ingest is a two-step, file-format-agnostic
  flow (CSV/XLSX), both gated by **owner/admin**: `POST …/catalogue/import/inspect`
  returns the detected headers + a row sample; the SPA mapping wizard maps each
  source header to a target field; `POST …/catalogue/import/commit` applies it,
  upserting by `code` through the copy-on-write rules. The pipeline is
  `importer.ApplyMapping` (header→field → typed rows) feeding the `catalogue`
  slice's `Inspect` / `ImportMapped`.

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
