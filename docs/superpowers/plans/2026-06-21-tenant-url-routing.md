# Functional tenant-in-URL routing — Implementation Plan

> **For agentic workers:** This plan is executed by the **Ralph Loop** (autonomous, in-session). Work top-to-bottom; do not start a phase until the previous phase's **Exit gate** is green. Steps use checkbox (`- [ ]`) syntax. After each task: run its checks, then `git commit` on `main` (we work on `main` directly per the user). Keep `go vet`/`gofmt`/`go test ./...` and `cd web && npm run check` clean.

**Goal:** Make the active tenant part of the URL end-to-end — `/{tenantUUID}/…` in the browser and `/api/t/{tenantUUID}/…` on the API — so every page/table item is a bookmarkable, navigable, tenant-scoped path (shifts included), and a multi-tenant user switches tenant by navigating.

**Architecture:** Session identity becomes the **email**. A new `ResolveTenant` chi middleware on an `/api/t/{tenantUUID}` group authorizes the email against the URL tenant (`GetUserByTenantEmail`), resolves that tenant's user id + role into `reqctx`, and everything downstream stays `reqctx`-scoped. The SvelteKit SPA moves all authed routes under `src/routes/[tenant]/…`; the API client prepends the active tenant (computed per request) at one chokepoint; a tenant switcher + redirects complete the loop. Shifts becomes a full edit page like the other entities.

**Tech Stack:** Go (chi, scs sessions, sqlc, modernc sqlite), SvelteKit `adapter-static` SPA (Svelte 5 runes, TS), Vitest. No new deps.

**Spec:** `docs/superpowers/specs/2026-06-21-tenant-url-routing-design.md` — read it first.

**Working branch:** `main` (direct).

---

## File map

| File | Change |
|------|--------|
| `internal/db/queries/users.sql` | + `GetUserByTenantEmail :one` (full row, has role) |
| `internal/db/gen/*` | regen via sqlc |
| `internal/auth/users.go` | + `GetByTenantEmail` wrapper returning `*User` |
| `internal/httpx/middleware.go` | split `RequireAuth` → `RequireSession` (no tenant) + new `ResolveTenant` (URL tenant → authorize → reqctx tenant/user/role) |
| `internal/app/auth_handlers.go` | store `email` in session at login; add `Session` handler (`/auth/session`) |
| `internal/app/server.go` | restructure `/api`: agnostic authed group + `/api/t/{tenantUUID}` group under `ResolveTenant` |
| `web/src/lib/api/client.ts` | `setActiveTenant`/`activeTenant`; per-request tenant path helper |
| `web/src/lib/api/crud.ts` | build tenant-prefixed path **per request** |
| `web/src/lib/api/*.ts` (shifts, etc.) | prefix tenant-scoped calls |
| `web/src/lib/realtime/events.ts` | add `closeEvents()`/reopen at tenant URL |
| `web/src/lib/stores/session.svelte.ts` | `loadSession()` (email+tenants) + `loadMe()` (per-tenant) |
| `web/src/lib/nav.ts` (new) | `t(path)` tenant-prefix helper |
| `web/src/routes/**` | move authed routes under `src/routes/[tenant]/…`; add `[tenant]/+layout`; root redirect; tenant switcher |
| `web/src/routes/[tenant]/shifts/[id]/+page.svelte` (new) | full shift edit page |

---

# PHASE 1 — Backend: tenant from URL

## Task 1.1: `GetUserByTenantEmail` query + repo method

**Files:** `internal/db/queries/users.sql`, `internal/auth/users.go`, regen `internal/db/gen`.

- [ ] **Step 1:** Add to `internal/db/queries/users.sql`:
```sql
-- name: GetUserByTenantEmail :one
SELECT * FROM users WHERE tenant_id = ? AND email = ?;
```
- [ ] **Step 2:** Regenerate: `"$(go env GOPATH)/bin/sqlc" generate` (run from repo root). Expect `internal/db/gen/users.sql.go` to gain `GetUserByTenantEmail`.
- [ ] **Step 3:** Add a repo wrapper in `internal/auth/users.go` next to `GetByID`:
```go
// GetByTenantEmail resolves the user row (with role) for an email within a
// tenant. Returns (nil, nil) when the email has no account in that tenant —
// the caller treats that as "not a member" (403).
func (r *UsersRepo) GetByTenantEmail(ctx context.Context, tenantID int64, email string) (*User, error) {
	row, err := gen.New(r.db).GetUserByTenantEmail(ctx, gen.GetUserByTenantEmailParams{TenantID: tenantID, Email: email})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by tenant+email: %w", err)
	}
	u := toUser(row) // reuse whatever mapping GetByID uses (match its conversion)
	return &u, nil
}
```
Match the exact row→`*User` conversion that `GetByID` uses (copy its body). Confirm imports (`database/sql`, `errors`, `fmt`).
- [ ] **Step 4:** `go build ./... && go vet ./...` clean.
- [ ] **Step 5:** Commit: `git commit -am "feat(auth): query user by (tenant,email) for url-tenant resolution"`

## Task 1.2: store email in the session at login

**Files:** `internal/app/auth_handlers.go`.

- [ ] **Step 1:** In the login handler where it does `h.sm.Put(... "userID" ...)` / `"tenantID"`, also `h.sm.Put(r.Context(), "email", creds... /* the resolved email */)`. Use the email the credentials were resolved for (the request email, lowercased consistently with how lookups compare). If `Credentials` lacks email, use the validated request email.
- [ ] **Step 2:** `go build ./...` clean.
- [ ] **Step 3:** Commit: `git commit -am "feat(auth): persist email in session for per-request tenant auth"`

## Task 1.3: `ResolveTenant` middleware (TDD — security core)

**Files:** `internal/httpx/middleware.go`, test `internal/httpx/resolvetenant_test.go` (or extend existing middleware test file).

- [ ] **Step 1: Write failing tests.** Create `internal/httpx/resolvetenant_test.go`. Use the existing test helpers/patterns in the package (look at how the current middleware tests build an `scs` manager + fake repos; mirror them). Cover:
  - member email + valid tenant UUID in URL → handler runs; `reqctx.MustTenant` == that tenant; `reqctx.UserFrom` has the per-tenant user + role.
  - email not a member of the URL tenant → **403**, handler NOT run.
  - unknown tenant UUID → **404**.
  - suspended tenant → blocked (403), handler NOT run.
  - role is the URL tenant's role (set up the fake user repo so the same email has different roles in two tenants; assert each).
  If the package's auth repos are concrete (`*auth.UsersRepo`/`*auth.TenantsRepo`) and hard to fake, introduce small interfaces in `httpx` for exactly the methods `ResolveTenant` needs (`GetByTenantEmail`, tenant `GetTenantByUUID`/`Status`) and have the middleware accept those — mirror how `RequireAuth` currently takes its deps. Keep it minimal.
- [ ] **Step 2:** Run: `go test ./internal/httpx/ -run ResolveTenant -v` → FAIL (undefined `ResolveTenant`).
- [ ] **Step 3: Implement.** In `middleware.go`:
  - Add `RequireSession(sm)` — a lightweight gate: 401 unless the session has `userID` (and `email`); attach acting user id + email to context/logger. (Used by the tenant-agnostic authed group.)
  - Add `ResolveTenant(sm, users, tenants)` for the `/api/t/{tenantUUID}` group:
    1. `email := sm.GetString(ctx,"email")`; if empty → 401.
    2. `uuid := chi.URLParam(r, "tenantUUID")`; `tenant := tenants.GetByUUID(ctx, uuid)`; not found → 404.
    3. suspended → destroy/deny 403 (reuse current suspended logic).
    4. `u := users.GetByTenantEmail(ctx, tenant.ID, email)`; `u == nil` → **403 "forbidden"**.
    5. `ctx = reqctx.WithTenant(ctx, tenant.ID)`; `reqctx.WithUser(ctx, u.ID)`; `context.WithValue(ctx, userCtxKey, u)`; enrich logger with tenant_id/user_id. `next.ServeHTTP`.
  - Keep `RequireRole` unchanged — it reads `userCtxKey`, which now carries the per-tenant user/role. 
  - You may keep `RequireAuth` for now (unused after server.go rewires) or delete it in Task 1.4; don't leave it half-wired.
- [ ] **Step 4:** `go test ./internal/httpx/ -run 'ResolveTenant|RequireRole' -v` → PASS.
- [ ] **Step 5:** Commit: `git commit -am "feat(httpx): ResolveTenant middleware authorizes url tenant by email"`

## Task 1.4: rewire the router + `/auth/session`

**Files:** `internal/app/server.go`, `internal/app/auth_handlers.go` (add `Session` handler).

- [ ] **Step 1:** Add `GET /auth/session` handler returning `{ email, tenants: [{uuid,name,role}] }` from `ListTenantsByEmail` joined with role. (Add a sqlc query if role isn't in `ListTenantsByEmail`; otherwise compute role per tenant.) Tenant-agnostic.
- [ ] **Step 2:** Restructure the authed section of `server.go`:
```go
api.Group(func(pr chi.Router) {
    pr.Use(httpx.RequireSession(deps.Session))
    if deps.Auth != nil {
        pr.Get("/auth/session", deps.Auth.Session) // NEW: email + tenants
    }
})
api.Route("/t/{tenantUUID}", func(tr chi.Router) {
    tr.Use(httpx.RequireSession(deps.Session))
    tr.Use(httpx.ResolveTenant(deps.Session, deps.Users, deps.Tenants))
    if deps.Auth != nil { tr.Get("/auth/me", deps.Auth.Me) }
    if deps.Invites != nil { tr.With(httpx.RequireRole("owner","admin")).Post("/invites", deps.Invites.Create) }
    if deps.Events != nil { tr.Get("/events", deps.Events.Stream) }
    // …move ALL the existing deps.X.Routes(tr) + /shifts/import + /features here verbatim…
})
```
  Keep the same nil-guards. Public routes (signup/login/logout/invites validate+accept) stay where they are. Note `/features` + `/auth/me` are now tenant-scoped.
- [ ] **Step 3:** Update any backend tests that hit `/api/<resource>` to use `/api/t/{uuid}/<resource>` (the `internal/app/*_test.go` suite). Use a real tenant UUID from the test's seeded tenant. This is the bulk of the churn — work through each failing test.
- [ ] **Step 4:** Gate: `go test ./... -race` PASS; `go vet ./...`; `gofmt -l .` empty; `CGO_ENABLED=0 go build ./cmd/tallyo`.
- [ ] **Step 5:** Commit: `git commit -am "feat(app): mount tenant-scoped api under /api/t/{tenantUUID}"`

### Phase 1 Exit gate
`go test ./... -race` green; vet/gofmt clean; cgo-free build OK; `ResolveTenant` tests prove member→200 / non-member→403 / unknown→404 / suspended→blocked / per-tenant role. **Do not start Phase 2 until green.**

---

# PHASE 2 — Frontend: `[tenant]` routing, API prefix, switcher

## Task 2.1: API client active-tenant + per-request prefix

**Files:** `web/src/lib/api/client.ts`, `web/src/lib/api/crud.ts`.

- [ ] **Step 1:** In `client.ts` add:
```ts
let _activeTenant: string | null = null;
export function setActiveTenant(uuid: string | null): void { _activeTenant = uuid; }
export function activeTenant(): string | null { return _activeTenant; }
/** Build a tenant-scoped API path; throws if no tenant is active (programmer error). */
export function tenantPath(resource: string): string {
	if (!_activeTenant) throw new Error('tenantPath: no active tenant set');
	return `/api/t/${_activeTenant}/${resource}`;
}
```
- [ ] **Step 2:** In `crud.ts`, compute the base **per request** (not at factory build):
```ts
export function createCrud<T, TInput>(resource: string): Crud<T, TInput> {
	const base = () => tenantPath(resource); // per-call, reads current active tenant
	return {
		list: async () => (await apiGet<T[]>(base())) ?? [],
		query: async (params) => (await apiGet<ListResult<T>>(`${base()}${toQueryString(params)}`)) ?? { rows: [], total: 0 },
		get: async (id) => must(await apiGet<T>(`${base()}/${id}`), `${resource} get`),
		create: async (input) => must(await apiPost<T>(base(), input), `${resource} create`),
		update: async (id, input) => must(await apiPut<T>(`${base()}/${id}`, input), `${resource} update`),
		remove: async (id) => { await apiDelete<void>(`${base()}/${id}`); }
	};
}
```
  Import `tenantPath` from `./client`.
- [ ] **Step 3:** `cd web && npm run check` → expect errors only where bespoke modules still use old paths (next task). crud.ts/client.ts themselves clean.
- [ ] **Step 4:** Commit.

## Task 2.2: prefix every tenant-scoped fetch (the sweep)

**Files:** `web/src/lib/api/shifts.ts`, `web/src/lib/realtime/events.ts`, `web/src/lib/stores/session.svelte.ts`, `web/src/lib/stores/businessProfile.svelte.ts`, `web/src/lib/stores/features.svelte.ts`, plus any other direct `apiGet/apiPost`.

- [ ] **Step 1:** `grep -rn "apiGet\|apiPost\|apiPut\|apiPatch\|apiDelete\|EventSource\|fetch(" web/src/lib web/src/routes` → enumerate every call. Classify each: tenant-scoped (→ `tenantPath(...)`) or agnostic (auth/session, login, logout, signup, invites → leave).
- [ ] **Step 2:** Update `shifts.ts` bespoke endpoints (`/shifts/import`, `/shifts/{id}/items`, `/divide`, etc.) to `tenantPath('shifts/...')`.
- [ ] **Step 3:** `events.ts`: parameterize the URL and add teardown:
```ts
export function openEvents(): void { ensureOpen(); }
export function closeEvents(): void { if (source) { source.close(); source = null; } }
```
  In `ensureOpen`, build the URL via `tenantPath('events')` (import it); guard if no active tenant. Export `closeEvents` so the layout can re-open on tenant change.
- [ ] **Step 4:** `session.svelte.ts`: split into `loadSession()` → `apiGet('/api/auth/session')` (email + tenants; agnostic) and `loadMe()` → `apiGet(tenantPath('auth/me'))` (per-tenant user/role). Expose `tenants` for the switcher.
- [ ] **Step 5:** `businessProfile`/`features` stores → `tenantPath('business-profile')` / `tenantPath('features')`.
- [ ] **Step 6:** `cd web && npm run check` → 0/0 (aside from route-move fallout handled next task).
- [ ] **Step 7:** Commit.

## Task 2.3: move authed routes under `[tenant]/` + layout + switcher

**Files:** `web/src/routes/**` (git mv), new `web/src/routes/[tenant]/+layout.svelte`, `web/src/lib/nav.ts`, switcher component.

- [ ] **Step 1:** `git mv` every authed route dir into `src/routes/[tenant]/` (the 8 entity pages + `[id]`, `settings/**`, `support-catalog`, and `+page.svelte` → `[tenant]/+page.svelte`). Leave `login`, `signup`, `accept-invite`, and the root `+layout.svelte` at the top. Keep the existing top `+layout.svelte` as the app shell; add a nested `[tenant]/+layout.svelte`.
- [ ] **Step 2:** Create `web/src/lib/nav.ts`:
```ts
import { page } from '$app/state';
export function t(path: string): string {
	const tenant = page.params.tenant;
	if (!tenant) throw new Error('t(): no tenant in route');
	return `/${tenant}${path.startsWith('/') ? path : '/' + path}`;
}
```
- [ ] **Step 3:** `[tenant]/+layout.svelte`: on mount, `loadSession()`; read `page.params.tenant`; if not in `session.tenants` → `goto('/login')` (or a picker). Else `setActiveTenant(page.params.tenant)`, `loadMe()`, and `openEvents()`. On tenant param change (an `$effect` on `page.params.tenant`): `setActiveTenant(newUuid)`, `closeEvents(); openEvents()`, `loadMe()`. Render the tenant **switcher** (dropdown of `session.tenants`; selecting → `goto('/' + uuid + '/')`).
- [ ] **Step 4:** Update ALL in-app links/`rowHref`/`newHref`/`backHref`/`goto` in the moved pages to use `t(...)` (e.g. `rowHref={(r)=>t('/invoices/'+r.id)}`, `newHref={t('/invoices/new')}`, `backHref={t('/invoices')}`). Update nav menu links in the shell layout.
- [ ] **Step 5:** Root redirect: in the top `+layout` (or a root `+page`), if authenticated and at `/`, `goto('/' + firstTenantUuid + '/')` using `loadSession()`. Post-login redirect (`login` page) → `/{chosenTenantUuid}/`.
- [ ] **Step 6:** `client.ts` 401/`handleUnauthorized` + `publicPaths` still fine (login is unprefixed). Verify deep-link to `/{notMyTenant}/…` → backend 403 → client should redirect; add handling if needed (403 on tenant routes → go to a member tenant or login).
- [ ] **Step 7:** Gate: `cd web && npm run check` 0/0; `npm run build`; `CGO_ENABLED=0 go build ./cmd/tallyo`.
- [ ] **Step 8:** Commit.

### Phase 2 Exit gate
`npm run check` 0/0; `npm run build` + cgo-free build OK. Manual: `/{myTenant}/invoices` loads; switcher changes data; `/` redirects; deep-link to a non-member tenant doesn't show data. **Do not start Phase 3 until green.**

---

# PHASE 3 — Shifts edit page + completeness loop

## Task 3.1: shifts full edit page

**Files:** new `web/src/routes/[tenant]/shifts/[id]/+page.svelte`, modify `web/src/routes/[tenant]/+page.svelte` (home shifts list), `web/src/lib/components/ShiftForm.svelte` (reuse its contents).

- [ ] **Step 1:** Create `[tenant]/shifts/[id]/+page.svelte`: `idParam = page.params.id==='new'?'new':Number(...)`, wrapped in `{#key idParam}` (per the 2026-06-20 pattern). Host the shift editor inline: the flat fields (note/date/status) via `createAutosave` (reuse the helper) or a bespoke form like recurring; line items + Divide-with-AI as the rich section. Reuse `ShiftForm.svelte`'s logic — either render `ShiftForm`'s contents inline or extract them. `/shifts/new` = create. Back link `t('/')` (home).
- [ ] **Step 2:** Home page (`[tenant]/+page.svelte`): `ShiftTable` row click → `goto(t('/shifts/'+id))`; "+ Add shift" / record buttons → `t('/shifts/new')` / `t('/shifts/'+id)`. Remove the `ShiftForm` modal usage from home.
- [ ] **Step 3:** `npm run check` 0/0; `npm run build`.
- [ ] **Step 4:** Commit.

## Task 3.2: completeness loop (every table → navigable tenant path)

**Files:** audit across `web/src/routes/[tenant]/**`.

- [ ] **Step 1:** For EACH table, verify row click + new go through `t(...)` to an existing route and a deep-link loads:
  - `invoices, estimates, participants, tax-rates, plan-managers, custom-items, recurring, shifts` → `/{t}/<e>/{id}` and `/{t}/<e>/new`.
  - `settings/users`: tenant-prefixed; add `/{t}/settings/users/{id}` ONLY if a genuine per-user edit page is warranted, else leave list-only.
  - `support-catalog`: tenant-prefixed, read-only (no item route).
- [ ] **Step 2:** `grep -rn "goto(\|href=\|rowHref\|newHref\|backHref" web/src/routes/[tenant]` → confirm no tenant-scoped target is missing the `t(...)`/`/${tenant}` prefix (ignore external/`/login` etc.).
- [ ] **Step 3:** Final gate (below).
- [ ] **Step 4:** Commit.

### Phase 3 / Final exit gate
- `cd web && npm run check` → 0 errors / 0 warnings.
- `cd web && npm test` → all pass (autosave tests green).
- `cd web && npm run build` succeeds.
- `go test ./... -race`, `go vet ./...`, `gofmt -l .` empty, `CGO_ENABLED=0 go build ./cmd/tallyo`.
- `grep` shows no un-prefixed tenant-scoped `apiGet/apiPost/EventSource` and no `ShiftForm` modal on the home page.
- Manual smoke: log in → land on `/{tenant}/`; click a row in every table → navigates to its item page; switcher swaps tenant + data + role; deep-link to a non-member tenant is blocked.

---

## Notes for the loop
- **Phases are strictly ordered.** Backend (Phase 1) must be green before the frontend can talk to `/api/t/{uuid}`. If a frontend task fails because the API shape isn't right, fix Phase 1, don't hack the frontend.
- **Security is the point.** Never weaken `ResolveTenant` to make a test pass — if a test expects data without membership, the test is wrong.
- **Single chokepoint.** All tenant-scoped fetches go through `tenantPath(...)`. A 404 in dev usually means a missed prefix.
- **Commit per task** on `main`. Keep the gates green between tasks.
