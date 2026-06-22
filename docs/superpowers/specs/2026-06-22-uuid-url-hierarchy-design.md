# UUID URL Scheme + Path Hierarchy (HTML UI) — Design

**Date:** 2026-06-22
**Status:** Approved (design), pending implementation plan
**Companion to:** [`2026-06-22-spa-to-go-htmx-design.md`](2026-06-22-spa-to-go-htmx-design.md) — that
spec defines the SPA→Go+HTMX rewrite; this one fixes the **URL scheme and route
hierarchy** for the new server-rendered HTML pages.

## Problem

In the current routing, only the tenant segment is a UUID
(`/t/{tenantUUID}/...`); every child resource is addressed by a **sequential
int64** (`/invoices/{id}`, `/shifts/{id}/items/{itemId}`). This is inconsistent
and leaks enumerable IDs in URLs that may leave the app (an invoice/estimate
link shared with a plan manager). We want the new HTML UI routes to:

1. Use **UUIDs for every entity** in the path (non-enumerable, stable, no count
   leak), consistent with the already-UUID tenant segment.
2. Follow a **consistent, shallow hierarchy** with predictable naming.

**Scope: HTML UI routes only.** The `/api/...` JSON tree keeps int IDs and is
out of scope (changing it would break the programmatic contract). The on-disk
tenant-DB file naming (already UUID, per recent commits) is unchanged.

## Decisions

### D1 — Full UUID via a public alias column (keep the int PK)

Add `uuid TEXT NOT NULL UNIQUE` to every tenant entity exposed in a URL. The
int64 PK is retained — all foreign keys, joins, audit references, and the entire
`/api` tree continue to use it. The UUID is purely the **public URL alias**: HTML
handlers resolve `uuid → row` (then operate on the int PK internally). No PK
surgery, no FK churn, `/api` unaffected.

- UUIDs are generated at insert time (random v4), stored as TEXT, `UNIQUE`
  indexed per tenant DB.
- This is intentionally an alias, not a PK swap: it is the smallest change that
  makes URLs opaque without touching the relational core or the JSON API.
- Accepted consequence: the same entity is addressed by `uuid` in HTML and by
  int `id` in `/api`. Acceptable — they are two surfaces; the API may adopt the
  uuid later if wanted, but that is not in scope here.

### D2 — Shallow hierarchy (best-practice REST URL nesting)

- **Top-level resources are flat:** `/invoices/{invoiceUUID}`,
  `/participants/{participantUUID}`. An entity that exists on its own lives at the
  root of the tenant tree.
- **Owned children nest exactly one level:** things that cannot exist without a
  parent — `/shifts/{shiftUUID}/items/{itemUUID}`,
  `/invoices/{invoiceUUID}/payments/{paymentUUID}`,
  `/invoices/{invoiceUUID}/lines/{lineUUID}`.
- **Never nest two levels.** `/participants/{uuid}/invoices/{uuid}/payments/{uuid}`
  is rejected — once you hold the leaf UUID you don't need its grandparent in the
  path, and every handler would drag params it never uses. Max depth = 2.
- **Relationships are filtered list views, not nested paths.** "This
  participant's invoices" = `/invoices?participant={participantUUID}`; the
  participant detail page links there. No `/participants/{uuid}/invoices` route.

### D3 — Naming conventions

- kebab-case collection names (existing convention: `tax-rates`,
  `plan-managers`, `custom-items`, `support-catalog`).
- Collection plural; member `/{entityUUID}`; create form `/new`; the **autosave
  detail page IS the edit page** (no separate `/edit`, per the HTMX spec's
  autosave model).
- Only **literal verbs** are static path words: `new`, `bulk-delete`,
  `bulk-status`, `status`, `pdf`, `convert`, `duplicate`, `generate`, `divide`,
  `import`, `lines`, `items`, `payments`, `search`.
- **Typed param names** — `{invoiceUUID}`, `{lineUUID}`, `{participantUUID}` —
  not bare `{id}`. Self-documenting in chi handlers and avoids ambiguity in
  nested routes.

### D4 — Support catalogue becomes a tenant-owned resource

The NDIS support catalogue moves from the **control DB** (globally seeded via a
generated migration) to a **per-tenant** resource that each tenant populates
itself. Rationale: removes the global seed/generation machinery and the
platform-admin special case; the catalogue becomes a normal tenant CRUD resource
gated by **owner/admin** like everything else.

- **Versions are kept.** Yearly price changes plus per-invoice price pinning
  (`line_items.catalog_version_id`) are real requirements — old invoices must not
  re-price when a tenant uploads a newer catalogue. Do **not** flatten to a
  single item table.
- **XLSX ingest UI is deferred.** Tables and routes are defined now; the actual
  upload/parse wiring (`/support-catalog/new`, `POST .../versions`) is a later
  task. `catalog.ParseXLSX` is retained for when it lands. Until then the
  catalogue UI is effectively read-only/empty.
- Accepted cost: the NDIS price guide is identical for every Australian
  provider, so on a multi-tenant deploy each tenant re-uploads the same XLSX.
  Fine for the self-hosted single-tenant case; mild duplication otherwise.

### D5 — Invites keyed by UUID for revoke

Invites currently key on `token` (a secret). Revoking by token would put the
secret in the URL/logs. Add a `uuid` column to invites and revoke by uuid:
`DELETE …/settings/users/invites/{inviteUUID}`. The token stays the
accept-link secret (`/accept-invite?token=…`), never used as a path id for
management.

## Full HTML route map

`{xUUID}` = entity uuid · literal words are static segments · **P** = full page
(GET render) · **F** = HTMX fragment · **A** = action (mutation).

### Public (no tenant)
```
GET   /login                         P
POST  /login                         A   multi-tenant email → re-render tenant chooser
POST  /logout                        A
GET   /signup                        P
POST  /signup                        A   → HX-Redirect /t/{tenantUUID}/
GET   /accept-invite?token=…         P
POST  /accept-invite                 A
GET   /                              A   resolve user's tenants → redirect single, chooser if many
```

### Tenant root
```
GET   /t/{tenantUUID}/                P   dashboard
GET   /t/{tenantUUID}/events          —   SSE stream (HTMX sse-connect)
```

### Standard resource pattern
Applies identically to `participants`, `plan-managers`, `tax-rates`,
`custom-items`, `recurring` (shown with `participants`; all paths under
`/t/{tenantUUID}/`):
```
GET    …/participants                          P   list (filter/sort/paginate → F on #rows)
GET    …/participants/new                       P   create form
POST   …/participants                           A   → HX-Redirect …/{participantUUID}
GET    …/participants/{participantUUID}         P   detail = autosave edit page
POST   …/participants/{participantUUID}         A   autosave (field-group + save-badge F)
DELETE …/participants/{participantUUID}         A   hx-delete
POST   …/participants/bulk-delete               A   checkbox form
```
Per-resource extra:
```
POST   …/recurring/{recurringUUID}/generate     A   → new draft invoice
```

### Invoices (owned children: lines, payments)
```
GET    …/invoices                               P   list
GET    …/invoices/new                            P
POST   …/invoices                                A
GET    …/invoices/{invoiceUUID}                  P   edit (autosave)
POST   …/invoices/{invoiceUUID}                  A   autosave
DELETE …/invoices/{invoiceUUID}                  A
POST   …/invoices/bulk-delete                    A
POST   …/invoices/bulk-status                    A
POST   …/invoices/{invoiceUUID}/status           A
GET    …/invoices/{invoiceUUID}/pdf              —   download
GET    …/invoices/{invoiceUUID}/lines/search?q=  F   NDIS picker menu
POST   …/invoices/{invoiceUUID}/lines            F   append row + recompute totals
DELETE …/invoices/{invoiceUUID}/lines/{lineUUID} F   remove row + recompute totals
GET    …/invoices/{invoiceUUID}/payments         F   list
POST   …/invoices/{invoiceUUID}/payments         A
DELETE …/invoices/{invoiceUUID}/payments/{paymentUUID}  A
```

### Estimates (owned child: lines)
List/new/create/edit/delete/bulk-* identical to invoices, plus:
```
POST   …/estimates/{estimateUUID}/status         A
POST   …/estimates/{estimateUUID}/duplicate      A   → new draft
POST   …/estimates/{estimateUUID}/convert        A   → invoice
GET    …/estimates/{estimateUUID}/pdf            —
GET    …/estimates/{estimateUUID}/lines/search?q= F
POST   …/estimates/{estimateUUID}/lines           F
DELETE …/estimates/{estimateUUID}/lines/{lineUUID} F
```

### Shifts (owned child: items)
```
GET    …/shifts                                  P   list (+ ?status=)
GET    …/shifts/suggestions                       F   billing suggestions
GET    …/shifts/to-record                         F   awaiting-record
GET    …/shifts/new                               P
POST   …/shifts                                   A
GET    …/shifts/{shiftUUID}                        P   edit (autosave)
POST   …/shifts/{shiftUUID}                        A
DELETE …/shifts/{shiftUUID}                        A
POST   …/shifts/{shiftUUID}/status                 A
POST   …/shifts/{shiftUUID}/divide                 A   AI; 503 if disabled
POST   …/shifts/import                             A   AI; 503 if disabled
GET    …/shifts/{shiftUUID}/items                  F
POST   …/shifts/{shiftUUID}/items                  F
PATCH  …/shifts/{shiftUUID}/items/{itemUUID}       F
DELETE …/shifts/{shiftUUID}/items/{itemUUID}       F
```

### Support catalogue (tenant-owned; ingest deferred)
```
GET    …/support-catalog                                       P   this tenant's versions list
GET    …/support-catalog/new                                    P   upload/create version form   [DEFERRED]
POST   …/support-catalog/versions                               A   ingest (multipart)/manual    [DEFERRED]
GET    …/support-catalog/versions/{versionUUID}                 P   items in version
DELETE …/support-catalog/versions/{versionUUID}                 A
GET    …/support-catalog/versions/{versionUUID}/items/new        P   add item                     [DEFERRED]
POST   …/support-catalog/versions/{versionUUID}/items            A                                [DEFERRED]
POST   …/support-catalog/versions/{versionUUID}/items/{itemUUID} A   edit (autosave)              [DEFERRED]
DELETE …/support-catalog/versions/{versionUUID}/items/{itemUUID} A                                [DEFERRED]
```

### Settings
```
GET    …/settings                       P   business profile form
POST   …/settings                       A   autosave
GET    …/settings/account               P
POST   …/settings/account               A
GET    …/settings/users                 P   users + invites
POST   …/settings/users/invites         A   create invite link
DELETE …/settings/users/invites/{inviteUUID}  A   revoke
```

### Cross-links (filtered lists, not nested routes)
```
participant's invoices  →  …/invoices?participant={participantUUID}
participant's shifts    →  …/shifts?participant={participantUUID}
```

## Data-model ripples (for the implementation plan)

- **Add `uuid TEXT NOT NULL UNIQUE`** to every tenant entity addressed in a URL:
  participants, plan-managers, tax-rates, custom-items, invoices, estimates,
  shifts, recurring, payments, invoice/estimate line items, shift items, invites.
  Backfill existing rows in the migration. Int64 PK retained throughout.
- **HTML handlers gain a `uuid → row` resolve step** (a `GetXByUUID` sqlc query
  per slice) before calling the existing service on the int id.
- **Catalogue tables move control DB → tenant DB:** `catalog_versions` + items +
  prices. New tenant migration; drop from control. `line_items.catalog_version_id`
  now references a tenant-local catalogue UUID.
- **Delete:** `cmd/cataloguegen`, the generated
  `internal/db/migrations/control/0000N_catalogue_*.sql`, `data/catalogue/`, and
  the platform-admin gate on catalogue ingest. Keep `catalog.ParseXLSX` for the
  deferred upload path.
- **Docs:** update `CLAUDE.md` (catalogue + DB sections) and `docs/data-model.md`
  ERD to move the catalogue from control to tenant and add the `uuid` columns.

## Out of scope

- `/api/...` JSON routes (stay int-keyed).
- Tenant-DB file naming (already UUID).
- The HTMX rendering mechanics, CSRF, SSE frame format — covered by the companion
  HTMX spec.
- XLSX catalogue ingest wiring (deferred; routes reserved).

## Testing

- `httptest` route tests asserting each HTML path resolves a UUID and 404s on an
  unknown/foreign-tenant UUID (the security check: a UUID from tenant A must not
  resolve in tenant B).
- A migration test asserting `uuid` is populated + unique on backfill for each
  table.
- Reuse existing service/repo tests (services operate on int PK, unchanged).
