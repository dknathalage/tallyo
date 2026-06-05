# Tallyo

Self-hosted, source-available (AGPL-3.0) invoice management web app — Go backend (chi + SQLite/modernc + sqlc) serving an embedded SvelteKit SPA.

> **Architecture note.** This was rewritten from an Electron + SvelteKit desktop app to a **single-binary Go web service** serving an embedded SvelteKit SPA. The Go implementation lives at the repo root (`cmd/`, `internal/`, `web/`); the legacy Electron/SvelteKit tree (`src/`, `electron/`, `drizzle/`, root `package.json`) is **superseded** and slated for removal. Design/plan docs: `docs/superpowers/{specs,plans}/`.

## Tech Stack

- **Backend:** Go 1.26 — `cmd/tallyo serve` (single binary). chi v5 router, REST JSON API.
- **Database:** SQLite via **modernc.org/sqlite** (pure-Go, no cgo) + **sqlc** (typed queries) + **goose** (embedded migrations, run on startup). `_txlock=immediate` DSN.
- **Auth:** email/password (bcrypt), server-side cookie sessions via `alexedwards/scs/v2` (SQLite-backed). Single-org, multi-user; first-run setup + manual invite links.
- **Realtime:** Server-Sent Events (`/api/events`) — an in-process hub broadcasts entity-change events; the SPA refetches into Svelte runes.
- **Frontend:** SvelteKit + `@sveltejs/adapter-static` (SPA, `200.html` fallback), Svelte 5 runes, Tailwind CSS 4, built and embedded via `//go:embed`.
- **PDF:** `johnfercher/maroto/v2` (pure-Go). **Import/Export:** stdlib `encoding/csv` + `xuri/excelize/v2` (pure-Go).
- **Testing:** Go stdlib `testing`; `svelte-check` + Vitest for the frontend.
- **License:** AGPL-3.0 (verbatim, copyleft) — `LICENSE`. The binary is **cgo-free** (`CGO_ENABLED=0 go build ./cmd/tallyo`).

## Project Layout

- `cmd/tallyo/main.go` — `serve` command: resolve data dir → open DB → migrate → build services → chi server (embedded SPA + `/api`) → graceful shutdown; runs the overdue + recurring sweeps (launch + hourly ticker).
- `internal/db/` — modernc connection (`sqlite.go`), `migrate.go` (goose), `migrations/*.sql`, `queries/*.sql` (sqlc source), `gen/` (sqlc output — do not edit).
- `internal/repository/` — data access per domain over sqlc gen; mutations routed through `audit.WithTx`.
- `internal/audit/` — `WithTx` audited-mutation wrapper + `Log`/`Changes`.
- `internal/numbering/` — concurrency-safe invoice/estimate document numbers (tx-scoped + retry).
- `internal/service/` — orchestration + SSE broadcast (commit-then-publish).
- `internal/http/` — chi server, middleware (recover/log/session/auth-guard), JSON helpers, static SPA handler, per-domain handlers, SSE `/api/events`.
- `internal/{pdf,export,importer,realtime,auth}/` — PDF render, CSV/XLSX export, catalog import (parse/map/diff/commit), SSE hub, auth (password/users/invites/session).
- `web/` — SvelteKit SPA (`src/lib/api`, `src/lib/stores`, `src/routes`); `web/embed.go` embeds `web/build`.

## Run

```bash
# Build the SPA first (the Go build embeds web/build):
cd web && npm install && npm run build && cd ..
# Run the server (single binary):
go run ./cmd/tallyo --port 8080            # or: go build -o tallyo ./cmd/tallyo && ./tallyo
# Frontend dev with hot reload (Vite proxies /api → :8080):
cd web && npm run dev
```
Flags: `--port`, `--data-dir` (else `DATA_DIR` env, else `os.UserConfigDir()/Tallyo`), `--secure-cookie` (behind TLS). DB file: `<data-dir>/tallyo-go.db`.

## Commands

- `go test ./...` — Go tests (add `-race` for the full gate).
- `go vet ./...` ; `gofmt -l .` — must be clean.
- `CGO_ENABLED=0 go build ./cmd/tallyo` — verify the cgo-free single binary.
- `"$(go env GOPATH)/bin/sqlc" generate` — regenerate `internal/db/gen` from `queries/*.sql`.
- `cd web && npm run check` — svelte-check (0 errors / 0 warnings) ; `npm run build` — emit `web/build`.

## Conventions

- sqlc source SQL in `internal/db/queries/`; never hand-edit `internal/db/gen/`.
- Handlers call **services**, services call **repositories**, repositories call sqlc gen — never skip layers.
- Every DB mutation is audited (via `audit.WithTx`) and broadcasts an SSE event from the service after commit.
- JSON is camelCase (Go struct json tags); list endpoints return `[]` (non-nil) when empty.
- Clean-break data model (fresh goose schema; no migration from the old Electron `tallyo.db`).
- Commits follow Conventional Commits.

## Database

- SQLite (modernc.org/sqlite, pure-Go) + sqlc + goose. WAL, `foreign_keys=ON`, `busy_timeout=5000`, `_txlock=immediate` (all mutations take the write lock at BEGIN).
- Migrations are embedded and run on startup (`internal/db/migrate.go`). Add a new migration as `internal/db/migrations/NNNNN_*.sql` then `sqlc generate`.
- DB file in the data dir (default `~/Library/Application Support/Tallyo/tallyo-go.db` on macOS); `DATA_DIR` / `--data-dir` override.

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
