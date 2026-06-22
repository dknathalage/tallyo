# UUID Path Scheme + Route Hierarchy — Design

**Date:** 2026-06-22
**Status:** Approved (design), pending implementation plan

> **Note.** The earlier HTMX/server-rendered rewrite is dropped. The app keeps
> the existing **SvelteKit SPA** served over the **`/api` JSON tree**. This spec
> covers only the UUID path scheme + route hierarchy across that SPA + API.

## Problem

Today only the tenant segment is a UUID (`/api/t/{tenantUUID}/...`,
`/:tenant/...` in SvelteKit); every child entity is addressed by a **sequential
int64** (`/invoices/{id}`, `/shifts/{id}/items/{itemId}`). This is inconsistent
and leaks enumerable IDs in URLs that can leave the app (an invoice/estimate link
shared with a plan manager). We want **every entity addressed by a UUID** in both
the SvelteKit routes and the `/api` paths, under a **consistent, shallow
hierarchy**.

## Architecture context

The SvelteKit SPA route `/:tenant/invoices/[invoiceUUID]` fetches
`GET /api/t/{tenantUUID}/invoices/{invoiceUUID}`. The same identifier flows
**client route → API path → server resolves to the int PK in the DB**. So the
SvelteKit routes and the API paths must use the *same* UUID; they mirror each
other.

## Decisions

### D1 — UUID is the only public identifier; int64 PK is internal-only

Add `uuid TEXT NOT NULL UNIQUE` to every tenant entity exposed in a URL or JSON
body. The int64 PK is **retained but never appears in any URL or API payload** —
it stays the DB-internal key for foreign keys, joins, and audit references. Every
public surface (SvelteKit route param, API path param, JSON `id`/`*Id` fields)
uses the UUID.

- UUIDs generated at insert (random v4), stored TEXT, `UNIQUE` per tenant DB.
- Server resolves `uuid → row` at the API boundary, then operates on the int PK
  internally. FK columns stay int; the service translates inbound `*Id` UUIDs to
  int FKs and outbound int FKs to UUIDs.

### D2 — Clean break: `/api` path params switch int → UUID

Only the SPA consumes the API, so the contract changes outright:
`/api/.../invoices/{id}` → `/api/.../invoices/{invoiceUUID}`. No dual int/uuid
resolution. Consistent with the project's clean-break data-model ethos.

**JSON-body ripple:** field *names* are kept to minimize SPA churn, but their
*values* become UUID strings — `id` holds the entity UUID; foreign-key fields
(`participantId`, `versionId`, …) hold the related entity's UUID. The server maps
these to/from int PKs at the service boundary.

### D3 — Shallow hierarchy (best-practice REST nesting)

- **Top-level resources flat:** `/invoices/{invoiceUUID}`,
  `/participants/{participantUUID}`.
- **Owned children nest exactly one level** — things that can't exist without a
  parent: `/shifts/{shiftUUID}/items/{itemUUID}`,
  `/invoices/{invoiceUUID}/payments/{paymentUUID}`.
- **Never nest two levels.** `/participants/{uuid}/invoices/{uuid}/payments/{uuid}`
  is rejected — once you hold the leaf UUID you don't need its grandparent.
  Max depth = 2.
- **Relationships are filtered list queries, not nested paths.** Participant's
  invoices = `GET …/invoices?participant={participantUUID}`; the SPA participant
  page links there. No `/participants/{uuid}/invoices` route.

### D4 — Naming conventions

- kebab-case collections (existing: `tax-rates`, `plan-managers`,
  `custom-items`, `support-catalog`).
- Collection plural; member `/{entityUUID}`. Only **literal verbs** are static
  segments: `bulk-delete`, `bulk-status`, `status`, `pdf`, `convert`,
  `duplicate`, `generate`, `divide`, `import`, `draft-from-shifts`, `lines`,
  `items`, `payments`, `prices`.
- **Typed param names** — `{invoiceUUID}`, `{lineUUID}`, `{participantUUID}` —
  not bare `{id}`; self-documenting in chi handlers and unambiguous when nested.

### D5 — Support catalogue becomes a tenant-owned resource

The NDIS catalogue moves from the **control DB** (globally seeded via generated
migration) to a **per-tenant** resource each tenant populates itself. Removes the
global-seed machinery and the platform-admin special case; the catalogue becomes
a normal tenant resource gated by **owner/admin**.

- **Versions kept.** Yearly price changes + per-invoice price pinning
  (`line_items.catalog_version_id`) are real — old invoices must not re-price.
  Don't flatten to a single item table.
- **XLSX ingest is deferred.** Tables + routes defined now; the upload/parse
  wiring is a later task. `catalog.ParseXLSX` retained for then. Catalogue UI is
  effectively read-only/empty until ingest lands.
- Accepted cost: identical NDIS guide per tenant means each re-uploads the same
  XLSX on a multi-tenant deploy. Fine for self-hosted single-tenant.

### D6 — Invites revoked by UUID

Invites key on `token` (a secret). Add a `uuid` column and revoke by uuid
(`DELETE …/invites/{inviteUUID}`); the token stays the accept-link secret
(`/accept-invite?token=…`), never a management path id.

## API route map (`/api`, the contract)

All under `/api/t/{tenantUUID}/` unless noted. `{xUUID}` = entity uuid; literal
words are static segments.

### Public (no tenant)
```
POST  /api/signup
POST  /api/auth/login                      (multi-tenant email → tenant list in body)
POST  /api/auth/logout
GET   /api/auth/session
GET   /api/invites/{token}                 validate accept-link (token, not uuid)
POST  /api/invites/{token}/accept
GET   /healthz
```

### Standard resource (participants, plan-managers, tax-rates, custom-items, recurring)
Shown with `participants`:
```
GET    …/participants
POST   …/participants
POST   …/participants/bulk-delete
GET    …/participants/{participantUUID}
PUT    …/participants/{participantUUID}
DELETE …/participants/{participantUUID}
```
Per-resource extras:
```
GET    …/participants/{participantUUID}/stats
POST   …/recurring/{recurringUUID}/generate
```

### Invoices (owned children: payments)
```
GET    …/invoices                                  (+ ?participant= / ?status=)
POST   …/invoices
POST   …/invoices/draft-from-shifts                (body: shift UUIDs)
POST   …/invoices/bulk-delete
POST   …/invoices/bulk-status
GET    …/invoices/{invoiceUUID}
PUT    …/invoices/{invoiceUUID}
DELETE …/invoices/{invoiceUUID}
POST   …/invoices/{invoiceUUID}/status
GET    …/invoices/{invoiceUUID}/pdf
GET    …/invoices/{invoiceUUID}/payments
POST   …/invoices/{invoiceUUID}/payments
DELETE …/invoices/{invoiceUUID}/payments/{paymentUUID}
```
(Line items stay embedded in the invoice PUT body as a `lineItems` array — each
carries its own `id` = lineUUID; no per-line endpoints, the SPA owns the array.)

### Estimates
List/CRUD/bulk identical to invoices, plus:
```
POST   …/estimates/{estimateUUID}/status
POST   …/estimates/{estimateUUID}/duplicate
POST   …/estimates/{estimateUUID}/convert
GET    …/estimates/{estimateUUID}/pdf
```

### Shifts (owned child: items)
```
GET    …/shifts                                    (+ ?status= / ?participant=)
GET    …/shifts/suggestions
GET    …/shifts/to-record
POST   …/shifts
POST   …/shifts/import                             (AI; 503 if disabled)
GET    …/shifts/{shiftUUID}
PUT    …/shifts/{shiftUUID}
DELETE …/shifts/{shiftUUID}
POST   …/shifts/{shiftUUID}/status
POST   …/shifts/{shiftUUID}/divide                 (AI; 503 if disabled)
GET    …/shifts/{shiftUUID}/items
POST   …/shifts/{shiftUUID}/items
PATCH  …/shifts/{shiftUUID}/items/{itemUUID}
DELETE …/shifts/{shiftUUID}/items/{itemUUID}
```

### Support catalogue (tenant-owned; ingest deferred)
```
GET    …/support-catalog/versions
POST   …/support-catalog/versions                  [DEFERRED] ingest/manual create
GET    …/support-catalog/versions/{versionUUID}/items
DELETE …/support-catalog/versions/{versionUUID}    [DEFERRED]
GET    …/support-catalog/items/{itemUUID}/prices
POST   …/support-catalog/versions/{versionUUID}/items            [DEFERRED]
PATCH  …/support-catalog/versions/{versionUUID}/items/{itemUUID} [DEFERRED]
DELETE …/support-catalog/versions/{versionUUID}/items/{itemUUID} [DEFERRED]
```

### Business profile & settings
```
GET    …/business-profile
PUT    …/business-profile
GET    …/auth/me
POST   …/settings/users/invites                    create invite link
DELETE …/settings/users/invites/{inviteUUID}       revoke
```

### Realtime
```
GET    …/events                                    SSE stream (unchanged)
```

## SvelteKit route mirror (file-based)

The client routes mirror the API hierarchy; `[uuid]` segments carry the same
UUID handed to the API. `/new` is a **client-only** create page (no API `/new` —
it POSTs to the collection).
```
/login   /signup   /accept-invite
/:tenant/                                  dashboard
/:tenant/{collection}                      list      (participants, invoices, …)
/:tenant/{collection}/new                  create form
/:tenant/{collection}/[entityUUID]         detail/edit
/:tenant/settings   /settings/account   /settings/users
/:tenant/support-catalog
```
Cross-links are filtered list pages: `/:tenant/invoices?participant={uuid}`.

## Data-model ripples (for the implementation plan)

- **Add `uuid TEXT NOT NULL UNIQUE`** to every tenant entity in a URL/payload:
  participants, plan-managers, tax-rates, custom-items, invoices, estimates,
  shifts, recurring, payments, invoice/estimate line items, shift items, invites.
  Backfill existing rows in the migration. Int64 PK retained, internal-only.
- **sqlc:** add `GetXByUUID` lookups and `uuid` to every select/insert. Inserts
  generate a UUID. FK columns stay int.
- **Handlers/services:** path param + `ParseID` → resolve uuid→row; map inbound
  `*Id` UUID body fields to int FKs and outbound int FKs to UUIDs. `ParseID`
  (currently int) gains/loses to a UUID parse helper.
- **Catalogue:** move `catalog_versions` + items + prices control DB → tenant DB
  (new tenant migration; drop from control). `line_items.catalog_version_id` now
  references a tenant-local catalogue UUID.
- **Delete:** `cmd/cataloguegen`, the generated
  `internal/db/migrations/control/0000N_catalogue_*.sql`, `data/catalogue/`, the
  platform-admin gate on ingest. Keep `catalog.ParseXLSX` for the deferred upload.
- **SPA:** API client types — `id`/`*Id` fields become UUID strings; route
  `[id]` params → `[uuid]`. Field names unchanged, so store/component churn is
  bounded.
- **Docs:** `CLAUDE.md` (catalogue + DB sections) and `docs/data-model.md` ERD —
  move catalogue control→tenant, add `uuid` columns.

## Out of scope

- Tenant-DB file naming (already UUID).
- XLSX catalogue ingest wiring (deferred; routes reserved).
- Any change to the SSE frame format or auth/session mechanics.

## Testing

- Handler tests: each API path resolves a UUID and **404s on an unknown or
  foreign-tenant UUID** (the core security check — a tenant A UUID must not
  resolve in tenant B's DB).
- Migration test: `uuid` populated + unique after backfill on every table.
- Round-trip test: create via POST returns a UUID `id`; GET by that UUID returns
  the same entity; FK fields round-trip as UUIDs.
- Reuse existing service/repo tests (services still operate on int PK).
