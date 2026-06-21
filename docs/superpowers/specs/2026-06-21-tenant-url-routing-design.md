# Functional tenant-in-URL routing (multi-tenant by path)

**Date:** 2026-06-21
**Status:** Approved (design)

## Problem

Tallyo is multi-tenant: one human (email) can have a user account in several
tenants, each with its own user row and role. Today the active tenant lives only
in the **session cookie** (chosen at login), and the backend scopes every `/api`
query by that session tenant. URLs carry no tenant. Consequences:

- A URL is not a complete address — `/invoices/5` means different records in
  different sessions; it can't be bookmarked or shared meaningfully.
- A multi-tenant user cannot view two tenants side by side or switch tenants
  without logging out and back in.
- One table (shifts) still opens a modal rather than a navigable page.

We want the **tenant to be part of the URL** and to **drive which tenant's data
loads**, end to end (browser address bar → SvelteKit route → `/api` call →
backend scoping), so that:

- every authed page lives at `/{tenantUUID}/…` and is bookmarkable/shareable;
- a multi-tenant user switches tenant by navigating (or via an in-app switcher);
- **every table's row click navigates to a real, tenant-prefixed, navigable
  path** — including shifts, which becomes a full edit page.

## Decisions (locked during brainstorming)

- **Functional** routing: the URL tenant drives data, not just cosmetics.
- **Tenant token = tenant UUID** (already a column; non-enumerable). Item ids
  stay numeric, as today (`/{tenantUUID}/invoices/5`).
- **API transport = path prefix**: tenant-scoped APIs under
  `/api/t/{tenantUUID}/…`; the validating middleware lives on that chi group so
  the boundary is structural. Tenant-agnostic endpoints stay at `/api/…`.
- **Identity = email**: the session's durable identity becomes the email; the
  per-tenant user id + role are resolved per request from the URL tenant.
- **Shifts** → full edit page on a route (`/{t}/shifts/{id}`), modal removed.
- **Tenant switcher** in the nav for multi-tenant users.
- **Loop scope:** the 8 entity tables (7 CRUD + shifts) get tenant-prefixed item
  routes; `settings/users` + `support-catalog` get the tenant prefix (users gets
  an item route only if a real per-user edit page is warranted; catalogue stays
  read-only, no item routes).
- **Execution:** implementation is driven to completion by the **Ralph Loop**, so
  the plan must have explicit, verifiable per-phase exit gates.

## Constraints / current architecture (verified)

- Backend: chi router, scs session (SQLite), `internal/reqctx` carries tenant +
  user. `RequireAuth` (`internal/httpx/middleware.go:84-136`) reads `userID` +
  `tenantID` from the session and attaches them via `reqctx.WithTenant`/`WithUser`;
  it re-validates the user via `GetUserByID(tenant_id, id)`.
- Users are unique per `(tenant_id, email)` (`00001_ndis_baseline.sql`). The
  login 409 "pick a tenant" flow is backed by `ListTenantsByEmail`
  (`internal/db/queries/users.sql`) returning `(tenant_id, tenant_name,
  tenant_uuid)`. `GetCredentialsForTenant(tenantID, email)` resolves a specific
  tenant's user.
- Every domain query takes `tenantID` from `reqctx.MustTenant(ctx)` and filters
  `WHERE tenant_id = ?`. Missing tenant panics.
- Routes: `r.Route("/api", …)` with a public section (signup/login/invites) and
  an authed `api.Group` guarded by `RequireAuth`; SPA fallback `r.Handle("/*",
  SPAHandler)`. (`internal/app/server.go`).
- Frontend: SvelteKit `adapter-static` SPA, Svelte 5 runes. `crud` factory builds
  `/api/${resource}` in ONE place (`web/src/lib/api/crud.ts`); bespoke modules:
  `shifts.ts`, the SSE `events` stream, auth. `session` store holds `User`
  (with `tenantId`). Item edit pages already exist for the 7 CRUD entities
  (route-based autosave work, 2026-06-20).

## Non-goals

- No tenant **slug** (UUID only). No new tenant naming/uniqueness scheme.
- No change to how tenants are created or how users are invited/provisioned.
- No cross-tenant data joins or "view all tenants at once" aggregate screens.
- No change to the NDIS catalogue's global/read-only model.
- No removal of the login-time tenant pick (it sets the initial URL tenant).

---

## Phase 1 — Backend: tenant resolved from the URL

### 1.1 Router shape

Split the authed `/api` group into tenant-agnostic and tenant-scoped:

```
/api
  POST /signup                      (public)
  POST /auth/login                  (public)
  GET  /invites/{token}             (public)
  POST /invites/{token}/accept      (public)
  group [RequireAuth]               (email identity only)
    GET  /auth/session              (email + member tenants — NEW)
    POST /auth/logout
  route /t/{tenantUUID} [RequireAuth, ResolveTenant]
    GET  /auth/me                   (per-tenant user + role)
    GET  /events                    (SSE, tenant-scoped)
    …all domain Routes(pr): invoices, estimates, participants, shifts,
      tax-rates, plan-managers, custom-items, recurring, business-profile,
      support-catalog, settings/users, etc.
```

### 1.2 Session identity = email

- At login (`internal/app/auth_handlers.go`), in addition to `userID`/`tenantID`,
  store `email` in the session. UX unchanged: a multi-tenant email still gets the
  409 pick-a-tenant flow; the chosen tenant becomes the initial redirect target.
- `RequireAuth` is relaxed to authenticate the **session** (email present, session
  valid) and attach the email/user identity; it no longer asserts a single tenant
  for tenant-scoped routes (that becomes `ResolveTenant`'s job). The existing
  tenant-agnostic group keeps minimal behavior.

### 1.3 `ResolveTenant` middleware (security core)

On the `/api/t/{tenantUUID}` group, after `RequireAuth`:

1. Read `{tenantUUID}` from the chi URL param.
2. Resolve the tenant by UUID → tenant row (404 if unknown). **Use the existing
   `GetTenantByUUID` query (`internal/db/queries/tenants.sql:9-10`) — no new
   query/migration needed.**
3. Reject if tenant status is suspended (moved from `RequireAuth`).
4. **Authorize + resolve role:** look up the user row for `(tenantID,
   sessionEmail)`. NOTE: `GetCredentialsForTenant` returns only
   `{ID, TenantID, Hash}` (`internal/auth/users.go:142-146`) — it has **no role**.
   So membership+role must come from the full user row (e.g. `GetUserByEmail` /
   the row carrying `Role` + `IsPlatformAdmin`, which is what `reqctx.UserFrom` /
   `RequireRole` already expect). No matching row ⇒ **403** (email not a member).
   Add a small `(tenant_id, email) → full user row` query if one doesn't already
   exist; do NOT rely on `GetCredentialsForTenant` for role.
5. Attach to context: `reqctx.WithTenant(tenantID)`,
   `reqctx.WithUser(thatTenantUserID)`, and the **per-tenant role/user object** the
   role gates read.
6. Downstream services/repos are unchanged — they still read
   `reqctx.MustTenant`/user as today.

### 1.4 Role gating

`RequireRole`/manager gates now read the **per-URL-tenant** role set by
`ResolveTenant` (correct: a user may be `owner` in tenant A, `member` in B).

### 1.5 `/auth/session` (new, tenant-agnostic)

`GET /api/auth/session` → `{ email, tenants: [{ uuid, name, role }] }` from
`ListTenantsByEmail` (+ role per tenant). Powers the SPA bootstrap and the tenant
switcher. `GET /api/t/{uuid}/auth/me` returns the per-tenant `User` (current
behavior, now under the tenant prefix).

### 1.6 Phase 1 exit gate

`go test ./... -race`, `go vet ./...`, `gofmt -l .` clean;
`CGO_ENABLED=0 go build ./cmd/tallyo` succeeds. New tests (see Testing) prove:
member email → 200; non-member email → 403; suspended tenant → blocked; role
resolved per tenant.

---

## Phase 2 — Frontend: `[tenant]` routing + switcher + API prefix

### 2.1 Route restructure

Move every authed route under `src/routes/[tenant]/…`:

- the 8 entity pages + their `[id]` routes, `settings/**`, `support-catalog`,
  and the home/shifts page (`+page.svelte` → `[tenant]/+page.svelte`).
- `login`, `signup`, `accept-invite` stay at the root (tenant-agnostic).

### 2.2 `[tenant]/+layout.svelte` (+ layout load)

- Read the active tenant UUID from `page.params.tenant`.
- On bootstrap, fetch `/api/auth/session`; validate the route tenant is in
  `tenants`. If not a member (or unknown) → redirect to a picker (or first member
  tenant / login).
- Publish the active tenant UUID to a module-level holder consumed by the API
  client (e.g. `setActiveTenant(uuid)` in `api/client.ts`).
- Render the **tenant switcher** (dropdown of member tenants; selecting one →
  `goto('/' + uuid + restOfPathOrHome')`).

### 2.3 API client prefix

- `crud` factory builds `/api/t/${activeTenant()}/${resource}` (one chokepoint).
  **Critical:** call `activeTenant()` **inside each request method**, not when the
  factory is constructed. The current `const base = '/api/${resource}'` is captured
  at `createCrud(resource)` call time (module load); freezing the tenant there
  would pin every store to whichever tenant was active first. Build the URL
  per-request.
- **Complete the sweep** — these tenant-scoped fetches all need the prefix, not
  just `crud`: `shifts.ts`, the SSE `events` URL, `businessProfile.svelte.ts`
  (`/api/business-profile`), `features.svelte.ts` (`/api/features`), and the
  session store's `refresh()` (`/api/auth/me` → now `/api/t/{uuid}/auth/me`).
  Auth/`session`/signup/invites stay unprefixed. The plan's grep step must
  enumerate every `apiGet`/`apiPost`/`EventSource`/`fetch` and classify each as
  tenant-scoped (prefix) or agnostic.
- Guard: if `activeTenant()` is unset when a tenant-scoped call is made, that's a
  programmer error (throw) — surfaces missing-prefix bugs early.
- **SSE singleton:** `web/src/lib/realtime/events.ts` holds one module-level
  `EventSource`; add a close/reopen API so the active-tenant change can tear down
  and re-open the stream at the new tenant-scoped URL (doesn't exist today).
- **Bootstrap ordering:** no tenant-scoped fetch may fire before the `[tenant]`
  layout load resolves membership and calls `setActiveTenant`. Page data loads run
  in/after the layout, not in parallel root loaders.

### 2.4 Links + redirects

- A `t(path)` helper prefixes in-app links: `t('/invoices') → /{active}/invoices`.
  Update nav, `rowHref`, `newHref`, `backHref`, and any `goto(...)` for in-app
  navigation to go through it.
- `/` (root) → redirect to `/{currentTenant}/` (current = the session's chosen
  tenant from `/auth/session`, e.g. the first/most-recent). Post-login → redirect
  to `/{chosenTenant}/`. SPA `200.html` fallback unchanged.
- SSE: re-subscribe when the active tenant changes (the stream URL is
  tenant-scoped).

### 2.5 Phase 2 exit gate

`cd web && npm run check` 0/0; `npm run build` succeeds; `CGO_ENABLED=0 go build
./cmd/tallyo` (embeds build) succeeds. Manual: deep-link `/{myTenant}/invoices`
loads; deep-link `/{notMyTenant}/…` redirects/403s; switcher changes data; back
button works.

---

## Phase 3 — Shifts edit page + completeness loop

### 3.1 Shifts as a full edit page

- New `src/routes/[tenant]/shifts/[id]/+page.svelte`: hosts the existing
  `ShiftForm` contents inline on the route — record flow (scheduled→recorded),
  line items, **Divide with AI**, note/date. `/{tenant}/shifts/new` for create.
- Home shifts list (`ShiftTable` on `[tenant]/+page.svelte`): row click →
  `t('/shifts/' + id)`; "+ Add shift"/record buttons → the new route. Remove the
  `ShiftForm` modal usage from the home page.
- Reuse `createAutosave` for the flat shift fields where it fits (note/date), with
  the rich sections (line items / AI) managed as on the recurring/invoice pages.

### 3.2 Completeness loop (Ralph)

Iterate **every** table and assert each row click resolves to a real,
navigable, tenant-prefixed path:

`invoices, estimates, participants, tax-rates, plan-managers, custom-items,
recurring, shifts` → `/{t}/<e>/{id}`; `settings/users` (+ item route only if a
real edit page is warranted) and `support-catalog` (read-only) → tenant-prefixed.
For each: confirm `rowHref`/`newHref` use `t(...)`, the route exists, and a
deep-link loads under a valid tenant.

### 3.3 Phase 3 exit gate

`npm run check` 0/0; `npm run build`; full Go gate; `npm test`; grep shows no
un-prefixed in-app `goto`/`href` to tenant-scoped paths and no remaining
`ShiftForm` modal on the home page.

---

## Data flow (after)

```
Browser /{tenantUUID}/invoices/5
  → SvelteKit [tenant] layout: validate membership, setActiveTenant(uuid)
  → page loads via crud → GET /api/t/{uuid}/invoices/5
  → RequireAuth (email) → ResolveTenant (authorize email∈tenant, set tenant+user+role)
  → service/repo scope by reqctx tenant (unchanged)
Switcher → goto('/{otherUuid}/…') → layout re-resolves → API now hits /api/t/{otherUuid}/…
```

## Security & error handling

- **Cross-tenant access:** non-member email on `/api/t/{uuid}/…` → 403; never
  leaks data (membership checked before any query). Frontend mirrors with a
  redirect, but the backend 403 is the authority.
- **Unknown tenant UUID** → 404. **Suspended tenant** → blocked (existing
  behavior, relocated to `ResolveTenant`).
- **Deep link to a non-member tenant** → backend 403 / frontend redirect to a
  tenant the email *is* in (or login).
- **Role escalation:** role is resolved per URL tenant, so a `member` in tenant B
  cannot use a manager-only route in B even if `owner` in A.
- **Session fixation / stale identity:** unchanged scs behavior; logout clears
  email + selection.

## Testing

- **Backend (Go):** `ResolveTenant` unit/integration — member→200, non-member→403,
  unknown uuid→404, suspended→blocked, per-tenant role correct; one end-to-end
  test that the same email hitting two different `/api/t/{uuid}` prefixes gets
  each tenant's data + role. Existing domain tests unaffected (still
  `reqctx`-scoped).
- **Frontend:** `npm run check` 0/0; the existing `createAutosave` tests stay
  green; shift edit page compiles. Manual matrix: deep-link valid/invalid tenant,
  switcher, back/forward across tenants, SSE re-subscribe.
- **Gates:** Go race+vet+gofmt clean; cgo-free build; SPA build embeds.

## Risks / open notes

- **Auth refactor is the highest-risk change** (Phase 1) — identity moving from
  (tenant,user) to email with per-request re-resolution. Land it behind tests
  first; Phases 2–3 depend on it.
- **`settings/users` item route** is conditional — add only if a genuine per-user
  edit page exists/is wanted; otherwise prefix-only.
- **SSE** must re-subscribe on tenant switch or events will target the old tenant.
- **Every fetch must go through the prefix chokepoint** — a stray un-prefixed
  tenant-scoped call will 404 under the new router; the client-side guard
  (throw when `activeTenant` unset) catches most at dev time.
