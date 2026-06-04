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

SQLite moves from the skeleton's `SetMaxOpenConns(1)` to a small pool: WAL allows
concurrent readers with a single serialized writer (relying on `busy_timeout`).
Sized small (e.g. a handful of connections) — appropriate for a single-org
self-hosted server. Exact pool size validated during implementation.

## Auth, Users, Invites

### Storage (new goose migration)

- `users` — id, uuid, email (UNIQUE), password_hash, role (`owner` | `member`),
  created_at, updated_at, last_login_at (nullable).
- `invites` — id, token (random, UNIQUE), email, role, created_by (user id),
  expires_at, used_at (nullable until consumed).
- session table — owned/managed by the scs DB store.

### Password & sessions

- `golang.org/x/crypto/bcrypt` (default cost) for hashing; plaintext never stored.
- `alexedwards/scs/v2` sessions with a DB-backed store (survives restart).
  Cookie: httpOnly, SameSite=Lax, `Secure` when served over TLS. Server-side
  sessions → easy logout/revocation.

### Roles (minimal)

Two roles. `owner`/admin: manage users + invites. `member`: full
invoice-domain access, no user management.

### Flows

- **First-run setup:** while `users` is empty, all routes funnel to a setup
  screen that creates the first `owner`. Other routes redirect to setup until an
  owner exists.
- **Invite (no SMTP):** owner creates an invite → server returns a link with a
  token (`/accept-invite?token=…`) the owner copies and shares manually.
  Invitee opens it, sets a password → account created, token marked `used_at`.
  Tokens expire (default 7 days).
- **Login/logout:** email+password → session cookie; logout clears the session.
- **Auth guard:** all `/api/*` except login, setup, and invite-accept require a
  valid session; unauthorized → 401. SPA redirects to login on 401.

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
    to `index.html` for SPA client routing.
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

- **`web/`** — SvelteKit with `@sveltejs/adapter-static` (SPA fallback to
  `index.html`), Svelte 5 runes, Tailwind CSS 4.
- **Build + embed:** build to `web/build`; Go embeds via
  `//go:embed all:web/build` and serves through the static handler. Production =
  one binary.
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
2. Auth: first-run setup, login, session cookie, auth-guarded `/api`.
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
