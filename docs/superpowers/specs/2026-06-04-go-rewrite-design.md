# Tallyo Go Rewrite — Design

**Date:** 2026-06-04
**Status:** Approved (design phase)

## Motivation

Rewrite Tallyo from Electron + SvelteKit (Node) to a Go-backed desktop app.
Primary driver: **language preference** — move all business logic, persistence,
document generation, and import/export to Go; retire the Node server and the
SvelteKit server layer (`+page.server.ts`, `/api/*` routes).

Scope note: this moves the **backend** to Go. The **UI remains
TypeScript/Svelte** (Wails standard pattern). "Retire JS/TS" means the server and
data layer become Go — it does not eliminate the frontend's TypeScript.

The local-LLM AI chat feature (node-llama-cpp: AI chat, skills, sub-agents added
in commit `e892007`) is **dropped** from the rewrite. The general mutation
**audit log is kept** — it is not AI-specific.

## Desktop Shell Decision

**Wails v2.**

Evaluated webview-Go options: Wails v2, `webview/webview_go`, Lorca,
go-astilectron, Sciter, go-app. Wails v2 wins: mature, single ~10–20MB binary,
native OS window (WKWebView / WebView2 / WebKitGTK), auto-generated Go↔TS
bindings, electron-builder-style packaging for dmg/exe/AppImage.

Wails creates **one native OS window**. It does **not** expose a TCP port or run
a second process — unlike the current Electron setup, which runs a Node HTTP
server on a picked port (`get-port`) in-process.

### Frontend approach — the true standard Wails pattern (option A)

The most battle-tested Wails pattern is used: an embedded JS frontend bundle that
calls Go via **generated bindings** (`window.go.<service>.<Method>()`). No HTTP
server, no router, no server-side templating.

A pure-Go-UI alternative (Templ + HTMX via `AssetServer.Handler`) was considered
and rejected in favor of the proven standard path. Trade-off accepted: the UI
layer remains JavaScript, but **all substance (logic, DB, PDF, import/export) is
Go**.

**Frontend template: Svelte-TS** (Wails official template, Vite-based). Chosen to
reuse existing Svelte 5 knowledge and port current components with least
relearning. Tailwind CSS 4 carries over into the Vite frontend.

## Library Stack

| Concern        | Current (JS)         | Go pick                  | Rationale |
|----------------|----------------------|--------------------------|-----------|
| Desktop shell  | Electron             | **Wails v2**             | single binary, native window, TS bindings |
| Frontend       | SvelteKit            | **Svelte-TS** (Wails/Vite) | reuse Svelte 5, port components |
| SQLite driver  | better-sqlite3 (cgo) | **modernc.org/sqlite**   | pure Go — eliminates native-rebuild pain |
| Query layer    | Drizzle ORM          | **sqlc**                 | compile SQL → typed Go, no runtime ORM magic |
| Migrations     | drizzle-kit          | **goose** (embedded)     | run on startup like today |
| PDF            | jsPDF + autotable    | **maroto v2**            | high-level tables/invoices |
| CSV            | PapaParse            | **encoding/csv** (stdlib)| built-in |
| Excel          | xlsx (SheetJS)       | **excelize**             | de-facto Go xlsx |
| Validation     | zod                  | **explicit checks**      | NASA rule 5 — validate at boundaries |
| Styling        | Tailwind CSS 4       | **Tailwind CSS 4**       | carries over via Vite |

**cgo note:** Wails webview uses cgo on mac/linux for the native webview. The
**database** layer is pure-Go (modernc), which is where the better-sqlite3
rebuild pain lived. Net: DB rebuild hell removed; webview cgo is handled by Wails
tooling.

## Project Layout

```
tallyo/
  main.go                  # Wails boot, embed frontend dist
  app.go                   # App lifecycle; service binding registration
  wails.json
  internal/
    db/
      sqlite.go            # modernc connection, pragmas (WAL, foreign_keys)
      migrate.go           # goose embed, run on startup
      migrations/*.sql     # goose migration files
      queries/*.sql        # sqlc source SQL
      gen/                 # sqlc OUTPUT (typed Go) — generated, do not edit
    repository/            # interfaces + SQLite impls over sqlc gen
      invoice.go estimate.go client.go payer.go catalog.go
      payment.go tax_rate.go rate_tier.go recurring.go
      business_profile.go column_mappings.go dashboard.go
    numbering/             # invoice/estimate number sequence generation
                           #   (port of db/number-generators.ts +
                           #    utils/invoice-number.ts, utils/estimate-number.ts)
    recurring/             # recurring-invoice scheduling (see Recurring section)
    reports/               # reporting aggregation (port of reports queries)
    audit/                 # mutation audit-log wrapper
    pdf/                   # maroto invoice/estimate rendering
    importexport/          # encoding/csv + excelize + saved column mappings
    money/                 # currency / formatting helpers (port of utils)
  bindings/                # Wails-generated TS types (auto, do not edit)
  frontend/                # Svelte-TS Vite project
    src/lib/components/    # ported from current src/lib/components
    src/...                # pages / views
```

The `drizzle/` directory and Electron `electron/` directory go away.

## Architecture Boundary

The Wails binding boundary replaces the current Electron HTTP routes:

```
Svelte component
   │  await InvoiceService.Create(dto)   // Wails-generated TS binding
   ▼
bound Go service method
   │
   ▼
repository  ──►  audit  ──►  sqlc gen  ──►  modernc sqlite
   │
   ▼
returns Go struct ──► auto-marshaled ──► typed TS object in component
```

Wails auto-generates TS types for every exported Go struct and bound method, so
the frontend/backend contract is generated, not hand-maintained.

### Decisions

1. **Repository interface layer — kept.** Preserves the current rule "use
   repositories, never queries directly." Bound service methods call
   repositories; repositories wrap sqlc gen + audit logging.
2. **Per-domain bound services**, rather than one monolithic `App` struct.
   Full set (one per shipped domain):
   `InvoiceService`, `EstimateService`, `ClientService`, `PayerService`,
   `CatalogService`, `PaymentService`, `TaxRateService`, `RateTierService`,
   `RecurringService`, `BusinessProfileService` (settings), `DashboardService`,
   `ReportService`, `ImportExportService`, `AuditService`.
3. **Tailwind CSS 4** retained in the Vite frontend.

## Data & Migrations

- **SQLite file location.** Wails v2 has no Electron-style
  `app.getPath('userData')` helper. Derive the data dir in Go from
  `os.UserConfigDir()` + `/Tallyo`, matching the current macOS path
  `~/Library/Application Support/Tallyo/tallyo.db`. Honor a `DATA_DIR` env
  override as today.
- **Migrations on startup** via embedded goose (mirrors current
  `electron/main.cjs` migrate-on-boot).
- **sqlc ↔ modernc wiring (implementation risk — must verify first).** sqlc's
  SQLite engine emits `database/sql` code with `?` placeholders. The Go driver
  is registered as `modernc.org/sqlite` (driver name `"sqlite"`), not
  `mattn/go-sqlite3`. The skeleton step must confirm: (a) sqlc `engine: "sqlite"`
  output compiles against the modernc driver, (b) any needed sqlc type-overrides
  for SQLite columns, (c) param style matches. This is the first thing the
  implementation plan validates with a spike before broad query porting.

## Existing-user Data Migration (DECISION NEEDED)

Current users have a populated `tallyo.db` migrated by **drizzle-kit** (tracked
in a `__drizzle_migrations` table). goose tracks state in `goose_db_version`.
Pointing goose at an existing drizzle-migrated DB needs a **baseline migration**:
goose migration `0001` must reproduce the *current* drizzle end-state schema and
be marked as already-applied on existing DBs (so goose does not try to recreate
existing tables), while running fully on fresh installs.

The dropped AI tables (`aiChatSessions`, `aiChatMessages`, `aiChatToolCalls`) are
simply **not recreated** on fresh installs; on existing DBs they are left in
place (or dropped by a later migration) — no data carried over.

**Open question for the user:** preserve existing users' data (baseline-migration
approach, more care) vs. clean break (new app, users start fresh)? Default
assumption in this spec: **preserve** existing data via baseline migration.

## Recurring Invoices Scheduling

The current SvelteKit Node server is always-on; the Go/Wails app only runs while
the window is open and has **no always-on background server**. Recurring-invoice
generation therefore needs an explicit model:

- **Run-on-launch sweep**: on app start, `RecurringService` generates any
  invoices whose `nextRunDate` has passed.
- **In-session ticker**: a bounded `time.Ticker` in `app.go` re-checks
  periodically while the app is open.

This is a behavior change from the always-on server and is called out so the
implementation plan handles it deliberately.

## Audit Logging

All database **mutations** remain audit-logged (current invariant). The audit
wrapper sits in the repository layer so every create/update/delete is recorded
regardless of which bound service triggered it.

## Coding Rules

The existing NASA Power-of-10 adaptation in `CLAUDE.md` continues to apply,
reinterpreted for Go:

- Simple control flow, flat early-return, no unbounded recursion.
- Bounded loops.
- Short functions (≤60 lines).
- Assertion density — validate inputs at boundaries (bound methods, DB, file
  I/O); `return fmt.Errorf(...)` / explicit error returns, never silent coercion.
- Check every return value — Go errors must be handled, never `_`-discarded on
  fallible calls.
- `go vet`, `staticcheck`, and `gofmt` clean on every commit (Go analog of the
  zero-warning rule).

## Testing

- Go: standard `testing` package, table-driven tests co-located as
  `*_test.go` next to query modules (mirrors current co-located `.test.ts`).
- Repositories tested against a real in-memory / temp-file modernc SQLite DB.
- Frontend: Vitest carries over for Svelte component tests.

## Distribution

- Wails `build` produces platform installers (dmg / exe / AppImage), replacing
  electron-builder.
- The existing `install.sh` (downloads the right installer per host from GitHub
  Releases) adapts to the new artifact names.
- GitHub Actions release matrix updated from `electron-release.yml` to a Wails
  build matrix.

## Known Risks

- **PDF layout fidelity.** maroto v2 is a grid/row builder, not a drop-in for
  jsPDF + autotable. Porting `utils/pdf.ts` (custom invoice layout, logo
  placement, multi-page line-item tables) will not be pixel-identical. The plan
  must decide whether exact visual parity is required or "close enough" is fine.
- **sqlc ↔ modernc** driver wiring (see Data & Migrations) — spike first.
- **Frontend port effort is large** (see Frontend Port below).

## Frontend Port

This is not a mechanical component copy. SvelteKit routing, `+page.server.ts`
load functions, form actions, and `/api/*` routes **all disappear** (Wails serves
a SPA). Affected surface to rewrite onto Wails bindings:

- ~15 console pages under `src/routes/(app)/console/*` (invoices, estimates,
  clients, payers, catalog, rate-tiers, recurring, reports, settings, + new/edit
  detail routes).
- ~16 `/api/*` route groups → replaced by `window.go.<Service>.<Method>()` calls.

Plan: introduce a client-side router (e.g. `svelte-spa-router` or
`@sveltejs/kit` static adapter constrained to client routing), move every
server-`load` into a binding call, and port presentational components mostly
as-is. The implementation plan scopes this per-page.

## Out of Scope (this rewrite)

- Local LLM AI chat, skills, sub-agents (dropped). The AI DB tables are not
  recreated on fresh installs.
- Any feature not present in the current app — this is a port, not a redesign.

## Migration Strategy (high level)

Feature-port, not big-bang switchover. Suggested order (refined in the
implementation plan):

1. Wails skeleton + DB connection (modernc) + goose baseline migration + **sqlc↔modernc spike**.
2. Business profile / settings (needed by PDF + invoices) + audit wrapper.
3. Core domain: clients, payers, catalog.
4. Tax rates + rate tiers (+ `catalog_item_rates` join).
5. Numbering (invoice/estimate sequence generation, transactional uniqueness).
6. Invoices + estimates (line items, money) + payments.
7. Recurring templates + scheduling model (run-on-launch sweep / ticker).
8. PDF generation (maroto).
9. CSV / Excel import-export + saved column mappings.
10. Dashboard + reports.
11. Audit log surfacing (UI).
12. Packaging + install.sh + CI (Wails build matrix).
```
