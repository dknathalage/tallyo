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
- New Go middleware `RequirePlatformAdmin` (mirrors the existing `ResolveTenant`
  in `internal/httpx`): rejects non-admin sessions with 403 before any handler.

## Backend

New leaf package `internal/admin` with handlers over the **control DB** (reuses
the existing subscription control-DB store + idempotent `apply`). No new DB
columns.

| Method | Path | Action |
|--------|------|--------|
| `GET` | `/api/admin/tenants` | List/search tenants: name, `subscription_status`, trial_ends_at, lapse date, user count |
| `GET` | `/api/admin/tenants/:id` | Tenant detail (above + audit trail) |
| `PATCH` | `/api/admin/tenants/:id/subscription` | Override `subscription_status` (comp / extend trial / force-active) — routed through the existing idempotent control-DB `apply` |
| `POST` | `/api/admin/tenants/:id/suspend` | Suspend (reversible) |
| `POST` | `/api/admin/tenants/:id/unsuspend` | Reverse suspend |
| `DELETE` | `/api/admin/tenants/:id` | Delete tenant (destructive) |

Every mutating call writes an `audit` entry (existing `internal/audit`).

## Frontend

SvelteKit, reusing the existing component library — **no new components unless a
gap appears**: `DataTable`, `Badge`, `Modal`, `Button`, `Field`, `Card`.

**Screens:**
1. **Tenant list** (`/admin`) — searchable `DataTable`. Columns: name, status
   (`Badge`), trial/lapse date, user count. Row click → detail.
2. **Tenant detail** (`/admin/[id]`) — status + override control (`Field` +
   `Button`), suspend/unsuspend, delete, audit trail list.

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
- Override goes through the **existing** idempotent `apply` — covered there; add
  a handler-level test that a PATCH lands the new status.
- Frontend: type-to-confirm modal blocks delete until the name matches (mirror
  existing `autosave.test.ts` / component test style).

## Explicitly skipped (YAGNI)

- Impersonation (dropped per decision).
- Per-tenant analytics / charts — add when there's a question they answer.
- Pagination beyond what `DataTable` already does — add at tenant-count pain.
- A separate admin auth system — `is_platform_admin` flag is enough.
