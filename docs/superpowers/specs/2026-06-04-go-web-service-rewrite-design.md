# Tallyo Go Web-Service Rewrite — Design

**Date:** 2026-06-04
**Status:** Approved (design phase)
**Supersedes:** the Wails desktop approach in `2026-06-04-go-rewrite-design.md`. The
Go data layer built by the Wails walking skeleton is **reused**; the Wails shell
is dropped.

## Motivation

Pivot from a Wails desktop app to a **self-hosted Go web service with a Svelte
frontend**, served as a single binary. Same invoice-management domain; new
delivery model: multi-user, browser-accessed, SaaS-style UX, real-time data sync.

## Decisions Summary

- **Delivery:** single Go binary. `tallyo serve` serves a JSON REST API **and** an
  embedded SvelteKit static build. Accessed via browser at `http://host:PORT`.
- **Tenancy:** single-org, multi-user. One deployment = one business. All users
  share the same business data; auth gates access. NO tenant scoping / no
  `org_id`.
- **Auth:** email + password (bcrypt). Server-side cookie sessions via
  `alexedwards/scs/v2` with a DB-backed store. No JWT.
- **User provisioning:** first-run setup creates the `owner`; owner generates
  **invite links (token)** handed over manually — no SMTP dependency.
- **Realtime:** Server-Sent Events (SSE). Server broadcasts change
  **invalidations**; client refetches the affected resource into Svelte 5 runes.
- **Frontend:** SvelteKit + `@sveltejs/adapter-static` (SPA), Svelte 5 runes,
  Tailwind CSS 4. Built and embedded into the Go binary.
- **License:** **AGPL-3.0, verbatim** (unmodified FSF text in `LICENSE`).
  Copyleft: modifications run as a network service must release source.

## Architecture

### Reused from the Wails skeleton (unchanged)

- `internal/db` — modernc.org/sqlite connection (WAL pragmas), embedded goose
  migrations, sqlc-generated typed queries. The sqlc↔modernc compatibility is
  already proven.
- `internal/audit` — `Log` helper writing `audit_log` rows, transaction-aware
  via the `Execer` interface.
- `internal/repository` — repository pattern over sqlc gen + audit, with
  transactional audited mutations.
- Clean-break DB (fresh schema via goose; old Electron `tallyo.db` untouched).

### Dropped

- Wails: `main.go`, `app.go`, `wails.json`, the Wails `frontend/` scaffold and
  `frontend/wailsjs` bindings. No native window, no JS↔Go bindings.

### New / changed

- **`cmd/tallyo/main.go`** — `serve` command. Flags/env: `--port`, `--data-dir`
  (honors `DATA_DIR`), TLS handled by an external reverse proxy (nginx/Caddy).
- **`internal/http`** — chi router, JSON REST, middleware, embedded static serving.
- **`internal/service`** — repurposed: orchestrates mutation → repository → audit
  → **SSE broadcast** (commit-then-publish). The realtime publish seam lives here.
  **Breaking change to reused code:** existing service methods use
  `context.Background()`; they must change to take `ctx context.Context` so HTTP
  handlers pass `r.Context()` (cancellation + SSE lifecycle). Not a no-op reuse.
- **`internal/auth`** — password hashing, sessions, users, invites.
- **`internal/realtime`** — in-process SSE hub + `Event` type.
- **`web/`** — new SvelteKit static app, embedded into the binary.

### Data flow (mutation)

```
Svelte (POST /api/invoices) → handler → service
   → repository → audit → sqlc → modernc (tx commit)
   → hub.Broadcast({entity:"invoice", id, action})       // ONLY after commit
SSE /api/events ─push─► all clients ─► sync layer refetches invoice → runes update
```

### Concurrency

**This requires MODIFYING the reused `internal/db/sqlite.go`** — it currently
hardcodes `SetMaxOpenConns(1)`. A web server has real concurrency the desktop app
never did. Change to a small pool: WAL allows concurrent readers with a single
serialized writer. Config: a handful of read connections + `busy_timeout` (already
set) so writers wait rather than erroring `SQLITE_BUSY`; serialize writes (modernc
has historically needed care — consider `_txlock=immediate` for write txns). Add
test coverage for concurrent read/write against modernc. Exact pool size validated
during implementation. NOTE: this is a code change, not reuse-as-is.

## Auth, Users, Invites

### Storage (new goose migration)

- `users` — id, uuid, email (UNIQUE), password_hash, role (`owner` | `member`),
  created_at, updated_at, last_login_at (nullable).
- `invites` — id, token (random, UNIQUE), email, role, created_by (user id),
  expires_at, used_at (nullable until consumed).
- `sessions` — **a goose migration must create this table**; scs's
  `sqlite3store` does NOT create it. Use scs's documented schema
  (`token TEXT PRIMARY KEY, data BLOB NOT NULL, expiry REAL NOT NULL`) + an index
  on `expiry`.

### Password & sessions

- `golang.org/x/crypto/bcrypt` (default cost) for hashing; plaintext never stored.
- `alexedwards/scs/v2` sessions with `sqlite3store.New(db *sql.DB)` — accepts the
  caller's `*sql.DB`, so it works with the modernc `"sqlite"` driver. `New()`
  starts a 5-minute cleanup goroutine; call `StopCleanup()` on shutdown.
  Cookie: httpOnly, SameSite=Lax, `Secure` when served over TLS. Server-side
  sessions → easy logout/revocation.
- **No password reset path** (no SMTP). Recovery = owner deletes + re-invites the
  user, or an out-of-band manual reset. Explicitly deferred.

### Roles (minimal)

Two roles. `owner`/admin: manage users + invites. `member`: full
invoice-domain access, no user management.

### Flows

- **First-run setup:** while `users` is empty, a setup guard allows only the
  setup route + static assets; protected routes return a "setup required" signal
  the SPA routes to the setup screen. Enforcement uses a **cached "owner exists"
  flag** (avoids `COUNT(*) users` per request), flipped after setup succeeds.
  `POST /api/setup` creates the first `owner` **inside a transaction** and is
  **rejected with 409** once any user exists (guards the race / double-submit).
- **Invite (no SMTP):** owner creates an invite → server returns a link with a
  token (`/accept-invite?token=…`) the owner copies and shares manually.
  Invitee opens it, sets a password → account created, token marked `used_at`.
  Tokens expire (default 7 days).
- **Login/logout:** email+password → session cookie; logout clears the session.
- **Auth guard:** all `/api/*` except login, setup, and invite-accept require a
  valid session; unauthorized → 401. SPA redirects to login on 401. The guard
  stores the user id in the session and **re-checks the user still exists** each
  request, so deleting a user immediately invalidates their active session(s)
  (deleted user can't keep acting until expiry).

### Endpoints

`POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`,
`GET /api/setup/status`, `POST /api/setup`, `POST /api/invites` (owner),
`GET /api/invites/:token` (validate), `POST /api/invites/:token/accept`,
`GET /api/users` (owner), `DELETE /api/users/:id` (owner).

## Realtime (SSE)

### Server — `internal/realtime`

- In-process **hub**: client registry of channels with `Subscribe()` /
  `Unsubscribe()` / `Broadcast(Event)`. Per-client buffer is **bounded**; on
  overflow the client is signaled to resync and/or dropped (no unbounded growth).
- `Event{ Entity string; ID int64; Action string }` — e.g.
  `{"invoice", 42, "update"}`.
- **`GET /api/events`** — SSE handler: auth-gated; `Content-Type:
  text/event-stream`; registers the client; streams events until disconnect;
  heartbeat comment ~every 25s to keep proxies open; honors request-context
  cancellation for cleanup.
- **SSE auth = same-origin session cookie.** `EventSource` cannot set
  `Authorization` headers, but it sends cookies automatically for same-origin
  requests. `/api/events` is same-origin with the embedded SPA, so the session
  cookie rides along and the normal auth-guard applies — do NOT attempt a bearer
  token on EventSource. In dev, the Vite proxy for `/api` must preserve
  same-origin/credentials so the cookie reaches `/api/events`.
- **Publish point:** in the service layer, **after** the transaction commits
  successfully. Never publish on rollback.

### Client — `web/src/lib/realtime`

- A singleton `EventSource('/api/events')` wrapper with the browser's built-in
  auto-reconnect. On (re)connect, trigger a **full resync** of active views to
  cover events missed while disconnected.
- A **sync registry**: domain stores register interest in an entity type; on a
  matching event the store refetches the affected resource (invalidation model)
  into its `$state` runes; components update reactively.
- Mutations use ordinary `fetch` to `/api/*`. The originating client also
  receives the SSE echo; an idempotent refetch keeps it consistent (no
  self-special-casing).

### Guarantees

Consistency over latency. Every change triggers an authoritative refetch so
clients converge on server truth even after missed/dropped events. Bounded
buffers + heartbeat + reconnect-resync prevent leaks and stuck clients.

## HTTP / API

- **Router:** chi.
  - `/api/*` — JSON REST per domain (invoices, estimates, clients, payers,
    catalog, payments, tax-rates, rate-tiers, recurring, settings, dashboard,
    reports, audit — ported in later plans).
  - `/api/events` — SSE.
  - `/*` — serve embedded SvelteKit static build; unknown non-API paths fall back
    to **`200.html`** (SvelteKit's documented SPA fallback — NOT `index.html`,
    which collides with the prerendered root route) for SPA client routing.
- **Middleware:** recovery (panic→500, process survives) → request logging →
  session load (scs) → auth guard on protected groups.
- **CSRF:** SameSite=Lax cookie + same-origin `/api`. Add a CSRF token only if
  cross-site access is ever introduced. No CORS (same origin).
- **Conventions:** JSON in/out; validate at the boundary (reject malformed →
  400 `{error}`); consistent `{ "error": "message" }` + correct status
  (400/401/403/404/409/500); no stack leaks to clients (log server-side);
  timestamps RFC3339; money as integer minor units (confirm against existing
  schema during port).

## Frontend

- **`web/`** — SvelteKit with `@sveltejs/adapter-static` (`fallback: '200.html'`,
  SPA mode), Svelte 5 runes, Tailwind CSS 4.
- **Build + embed:** build to `web/build`; Go embeds via
  `//go:embed all:web/build` (the `all:` prefix is required to include SvelteKit's
  `_app/` underscore-prefixed dirs) and serves through the static handler.
  Production = one binary. **Build-order trap:** `go:embed` fails to compile if
  `web/build` is missing/empty — commit a placeholder (e.g. `web/build/.gitkeep`)
  and ensure the frontend build runs **before** the Go build in CI.
- **Dev mode:** run the Vite dev server with `/api` proxied to the Go process for
  hot reload (two processes in dev, one in prod).
- **API client:** thin typed `fetch` wrapper in `web/src/lib/api`; on 401,
  redirect to login.
- **Stores:** rune-based domain stores in `web/src/lib/stores`, wired to the sync
  registry.

## License

`LICENSE` contains the **AGPL-3.0 verbatim** (unmodified standard FSF text).
`CLAUDE.md` is reworded from "open-source" to "AGPL-3.0 (copyleft)". Rationale:
anyone may use, modify, and even commercialize, but a modified version run as a
network service must release its complete source — closing the SaaS loophole.

## Project Structure

```
tallyo/
  LICENSE                       # AGPL-3.0 verbatim
  cmd/tallyo/main.go            # serve command: flags, boot, run
  internal/
    db/                         # KEEP (modernc, migrate, sqlc gen, queries)
      migrations/               # + new: users, invites, sessions
    repository/                 # KEEP + extend per domain
    audit/                      # KEEP
    service/                    # REPURPOSE: orchestrate + broadcast SSE
    auth/                       # NEW: password, sessions(scs), users, invites
    realtime/                   # NEW: SSE hub + Event
    http/
      server.go                 # chi router, embed web/build, middleware wiring
      middleware.go             # recovery, logging, session, auth-guard
      handlers/                 # per-domain JSON handlers + auth + sse + setup
  web/                          # NEW SvelteKit (adapter-static, Svelte 5, Tailwind 4)
    src/lib/api/                # typed fetch client (401 → login)
    src/lib/realtime/           # EventSource singleton + sync registry
    src/lib/stores/             # rune-based domain stores
    src/routes/                 # login, setup, accept-invite, app pages
```

Removed: `main.go`, `app.go`, `wails.json`, `frontend/` (Wails scaffold +
`wailsjs`). Wails dependency dropped from `go.mod`.

## Migration from the Wails Skeleton

The Go data layer (`internal/db`, `repository`, `audit`) carries over directly.
Delete the Wails files and `frontend/`. The skeleton's BusinessProfile slice
becomes the template for the new pattern: handler → service (+broadcast) →
repository → audit → sqlc.

### Carry-forward items still apply

From the prior skeleton review, settle in the first new plan: context
propagation (use `r.Context()` from HTTP now, replacing `context.Background()`),
an audit-tx wrapper helper, real audit `entity_id` + change-set fidelity, and the
`(nil,nil)` not-found convention.

## First Plan Scope — Walking Skeleton v2

Proves the NEW architecture end to end:

1. `cmd/tallyo serve` boots → chi router → serves an embedded SvelteKit static
   build.
2. Auth: goose migration for `users` + `invites` + scs `sessions` table;
   first-run setup (guarded, 409 once owner exists), login, scs session cookie,
   auth-guarded `/api` (with user-exists re-check).
3. One protected vertical slice: Business Profile settings over `/api`
   (GET/PUT) reusing the existing repository.
4. SSE hub + `/api/events` + client sync: a change in one browser tab reflects
   live in another.

Domains (clients, invoices, estimates, payments, etc.) are ported in subsequent
plans onto this pattern.

## Out of Scope (this rewrite)

- Multi-tenancy / org isolation.
- SMTP / email sending (invites are manual links).
- Local LLM AI chat (already dropped).
- Any feature not in the current app — this remains a port, not a redesign.

## Testing

- Go: stdlib `testing`; repositories against temp modernc DBs; HTTP handlers via
  `httptest`; auth (hashing, session, invite lifecycle) unit-tested; SSE hub
  tested for subscribe/broadcast/unsubscribe + bounded-buffer behavior.
- Frontend: Vitest for stores/sync logic and components.
```
