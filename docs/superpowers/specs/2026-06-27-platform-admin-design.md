# Platform Super-Admin Panel — Design

**Date:** 2026-06-27
**Branch:** `feat/admin-and-landing` (off `feat/saas-subscriptions`)
**Status:** Approved design, pending plan

## Purpose

Give the Tallyo operator (us) a UI to manage **all** tenants from one place:
see who's on trial/lapsed/active, manually override a tenant's subscription
status (comp, extend, force-active), and suspend or delete a tenant. Replaces
hand-running SQL against the control DB.

**Out of scope (dropped):** impersonation / login-as-tenant. Debug tenant views
another way. Removed to keep the security surface small and this branch
reviewable.

## Audience & Gating

Single class of user: a **platform admin**. The `users` table already carries
`is_platform_admin` (set via `UsersRepo.Create(..., isPlatformAdmin bool)`).
No new role machinery.

- Routes live at `/admin/*` — a top-level SvelteKit route group **outside**
  `[tenant]`, so tenant resolution, the subscription paywall, and the
  trial/lapse banner do **not** apply.
- Middleware `RequirePlatformAdmin` **already exists** (`internal/httpx/
  middleware.go:190`, tested at `middleware_test.go:85`) — currently used for the
  catalogue-admin/J7 ingest area. **Reuse it.** Update its stale doc comment
  (it claims "only for the catalogue-admin area") to reflect the broader use.
- It reads `UserFrom(ctx)`, which is populated by `RequireAuth` — so `/api/admin`
  must mount under a **`RequireAuth`-only** group (like the existing
  tenant-agnostic group at `server.go:101`), with `RequirePlatformAdmin` chained
  after. Without `RequireAuth` first, every admin request 401s. Pin the mount
  point in `server.go` as part of this work.

## Backend

New leaf package `internal/admin` with handlers over the **control DB**. No new
DB columns. Tenants are addressed by **public UUID** (the API never exposes int
PKs — `users.go:18-23` convention); the route segment is named `uuid` to match
the rest of the app (`page.params.uuid`).

| Method | Path | Action |
|--------|------|--------|
| `GET` | `/api/admin/tenants` | List tenants: name, `subscription_status`, trial_ends_at, lapse date, user count |
| `GET` | `/api/admin/tenants/:uuid` | Tenant detail (above + audit trail) |
| `PATCH` | `/api/admin/tenants/:uuid/subscription` | Override `subscription_status` |
| `POST` | `/api/admin/tenants/:uuid/suspend` | Suspend (reversible) |
| `POST` | `/api/admin/tenants/:uuid/unsuspend` | Reverse suspend |
| `DELETE` | `/api/admin/tenants/:uuid` | Delete tenant (destructive) |

### New backend work this requires (NOT just wiring)

Spec review found the existing stores do **not** cover these operations. Own
them explicitly:

1. **Subscription override — new method, do NOT reuse `subscription.Store.Apply`.**
   `Apply` (`store.go:46`) is idempotency-gated on Stripe `SyncedAt` and
   overwrites `StripeCustomerID`/`StripeSubscriptionID` — a manual override has
   no Stripe timestamp and would either no-op or clobber the Stripe linkage. Add
   a dedicated `SetSubscriptionStatus(ctx, tenantUUID, status, adminUserID)` that
   writes only `subscription_status` (+ `trial_ends_at` when extending) and an
   audit row, bypassing the sync gate and leaving Stripe IDs untouched.
   - Allowed `status` values are the real entitlement statuses
     (`entitlement.go:21`): `active`, `trialing`, `past_due` (entitled),
     `canceled`, `none` (not). "comp" = write `active`; "extend trial" = write
     `trialing` + a future `trial_ends_at`. There is no synthetic "comp" status.
   - **Known limitation (document, don't engineer around):** a subsequent Stripe
     webhook for that tenant can overwrite a manual override via `Apply`. The
     override is an operator stopgap, not a lock. `// ponytail:` an override-lock
     column is the upgrade path if webhooks fight overrides in practice.
2. **Tenant list / lifecycle — new `TenantsRepo` methods + sqlc queries.**
   `TenantsRepo` (`internal/auth/tenants.go`) today has only `Count`, `Create`,
   `Status`, `GetByUUID`, `Signup`. Net-new:
   - `List` returning name + subscription fields + per-tenant user count.
   - `Suspend`/`Unsuspend` — set/clear `StatusSuspended` (today that constant is
     only ever *read* by login/`ResolveTenant`; nothing sets it).
   - `Delete`.
   Each via new sqlc queries.

### Audit on cross-tenant admin actions

Every mutating call writes an `audit` entry (`internal/audit`). `audit.Log`
stamps `tenant_id`/`user_id` from `reqctx`. For a cross-tenant admin action,
stamp the **target tenant's** `tenant_id` and the **acting admin's** `user_id`,
so the row lands in the affected tenant's trail attributed to the operator. The
new methods pass these explicitly rather than relying on `reqctx`.

## Frontend

SvelteKit, reusing the existing component library: `DataTable`, `Badge`,
`Modal`, `Button`, `Field`, `Card`. One **new store** is needed (see below) —
not a new component.

`DataTable` (`web/src/lib/components/DataTable.svelte`) takes a backing store
with a server-side `query(ListParams)` contract and offers **per-column filters
+ sort**, not a single global search box. So:
- The admin list needs a **new admin-tenants store** implementing that
  `rows/total/loading/query` contract against `GET /api/admin/tenants` (mirror
  the existing `lib/stores/*.svelte.ts` stores).
- "Search" = the existing per-column text filter (e.g. filter by name). No new
  global-search affordance. If a global search box is wanted later, that's new
  work — flagged, not built.

**Screens:**
1. **Tenant list** (`/admin`) — `DataTable` over the new store. Columns: name,
   status (`Badge`), trial/lapse date, user count. Per-column filter + sort. Row
   click → detail.
2. **Tenant detail** (`/admin/[uuid]`) — status + override control (`Field` +
   `Button`, status picker limited to the allowed values above),
   suspend/unsuspend, delete, audit trail list.

**UX guards (from ui-ux-pro-max):**
- Destructive actions (suspend, delete) → confirmation `Modal` requiring the
  operator to **type the tenant name** to confirm. (`confirmation-dialogs`,
  `destructive-emphasis`: red, spatially separated from normal actions.)
- Success toast on every mutation (`success-feedback`). No silent success.
- `overflow-x-auto` wrapper on the table for narrow viewports (`data-table`).
- Visible focus rings already enforced globally in `app.css`.

## Phasing

1. **P1** — list + detail + subscription override (read + low-risk write).
2. **P2** — suspend / unsuspend / delete (destructive guards).

Each phase is independently shippable and reviewable.

## Testing

- Go: `RequirePlatformAdmin` rejects non-admin (403) and admits admin; each
  handler's happy path + audit write. Follow existing `_test.go` patterns in
  `internal/httpx` and `internal/subscription`.
- `SetSubscriptionStatus` writes the new status + audit row, leaves Stripe IDs
  untouched, and rejects statuses outside the allowed set. Handler-level test
  that a PATCH lands the new status.
- New `TenantsRepo.List`/`Suspend`/`Unsuspend`/`Delete` each get a store test
  (mirror `tenants_test.go`): List returns user counts; Suspend sets the status
  login/`ResolveTenant` blocks on.
- Frontend: type-to-confirm modal blocks delete until the name matches (mirror
  existing `autosave.test.ts` / component test style).

## Explicitly skipped (YAGNI)

- Impersonation (dropped per decision).
- Per-tenant analytics / charts — add when there's a question they answer.
- Pagination beyond what `DataTable` already does — add at tenant-count pain.
- A separate admin auth system — `is_platform_admin` flag is enough.
