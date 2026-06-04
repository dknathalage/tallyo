# Tallyo Go Rewrite — Design

**Date:** 2026-06-04
**Status:** Approved (design phase)

## Motivation

Rewrite Tallyo from Electron + SvelteKit (Node) to a Go-backed desktop app.
Primary driver: **language preference** — move business logic, persistence, and
document generation to Go; retire the Node server and JS/TS-heavy backend.

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
    audit/                 # mutation audit-log wrapper
    pdf/                   # maroto invoice/estimate rendering
    importexport/          # encoding/csv + excelize
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
2. **Per-domain bound services.** Bind `InvoiceService`, `EstimateService`,
   `ClientService`, `PayerService`, `CatalogService`, `ImportExportService`,
   etc., rather than one monolithic `App` struct.
3. **Tailwind CSS 4** retained in the Vite frontend.

## Data & Migrations

- SQLite file location: same semantics as today — a resolved data dir (Wails
  provides an app-data path per OS; e.g. macOS
  `~/Library/Application Support/Tallyo/tallyo.db`).
- Migrations run on app startup via embedded goose (mirrors current
  `electron/main.cjs` behavior of migrating on boot).
- `sqlc` generates typed query code from `queries/*.sql` against the schema;
  schema evolves through goose migrations.

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

## Out of Scope (this rewrite)

- Local LLM AI chat, skills, sub-agents (dropped).
- Any feature not present in the current app — this is a port, not a redesign.

## Migration Strategy (high level)

Feature-port, not big-bang switchover. Suggested order (refined in the
implementation plan):

1. Wails skeleton + DB connection + goose migrations + sqlc setup.
2. Core domain: clients, payers, catalog.
3. Invoices + estimates (numbering, line items, money).
4. PDF generation (maroto).
5. CSV / Excel import-export.
6. Dashboard.
7. Audit log surfacing.
8. Packaging + install.sh + CI.
```
