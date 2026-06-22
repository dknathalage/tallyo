# Generic Invoicing Core — NDIS as an Optional Capability

**Date:** 2026-06-23
**Status:** Design approved, pending spec review
**Branch (current):** feat/uuid-path-hierarchy

## Problem

Tallyo is hardwired to NDIS (National Disability Insurance Scheme) invoicing:
NDIS support-catalogue codes, price caps, pricing zones, plan windows, plan
managers, and GST-free defaulting are baked into the schema, the line-item
validator, and the UI. We are expanding beyond NDIS providers. The app must
serve **both** NDIS providers and generic goods-and-services providers from one
codebase, one schema, one deployment.

## Goal

A **generic invoicing core** where NDIS-specific behaviour is an *optional
capability* driven by **data presence**, not a global mode switch. NDIS users
keep their compliance machinery; generic users never see NDIS jargon or
constraints. No duplicated code paths.

## Non-Goals

- No data migration from existing databases. Clean-break schema (consistent with
  the project's existing clean-break stance — fresh goose schema).
- No new runtime dependencies.
- Multi-zone NDIS price-cap *import* (national/remote/very_remote → multiple
  prices) is out of scope; deferred as a follow-up. Schema retains nullable cap
  columns so it can be added without a migration redesign.
- Saved/reusable import mapping templates — deferred.
- Diff-against-existing import preview — unnecessary; the catalogue is versioned,
  so a new upload creates a new version and never mutates prior data.

## Core Principle

**Validation keys off data presence, not a mode flag.** A line is
catalogue-validated only if it references a catalogue item; a price cap is
enforced only if the item carries one; a plan window is asserted only if the
client has plan dates set. The single `client.type` enum exists for **UI and
required-field gating only** (which fields to show, which to require) — never as
a branch in the billing logic.

This keeps one code path. Generic data simply doesn't trigger NDIS rules.

## Renames (API + DB + UI, full rename)

| Old | New | Notes |
|---|---|---|
| `participant` slice/table | `client` | service recipient → generic client |
| `participants.ndis_number` | `clients.reference` | generic external reference string |
| `participants.mgmt_type` | `clients.mgmt_type` | kept, NDIS-only, nullable |
| `participants.plan_start/plan_end` | `clients.plan_start/plan_end` | kept, nullable |
| `plan_manager` slice/table/entity | `payer` | optional third party billed |
| `participants.plan_manager_id` | `clients.payer_id` | nullable FK to `payers` |
| `catalog` slice | `pricelist` (items) | generic priced items |
| `support_items` table | `items` | catalogue line items |
| `catalog_versions` table | `price_list_versions` | versioned releases |
| `support_item_prices` table | `item_prices` | zone caps kept, nullable |
| `support_items.support_category` etc. | `items.category` | collapse 3 NDIS columns → 1 optional text |
| `line_items.support_item_id` | `line_items.item_id` | FK to `items` |
| `line_items.catalog_version_id` | `line_items.price_list_version_id` | pinned version uuid |
| `line_items.gst_free` | `line_items.taxable` | inverted boolean |
| `shift` slice/table | `session` | generic billable session/work-log |
| `business_profile.zone` | `business_profile.zone` | kept, NDIS-only, nullable |

UI labels follow the DB/API renames. NDIS-only labels ("NDIS number", "NDIS
pricing zone", "Support catalogue") appear only when relevant (NDIS client type
/ when zone data exists).

> **UUID addressing unchanged.** Per the project convention, all renamed paths
> remain uuid-addressed (e.g. `/clients/{clientUUID}`, `/payers/{payerUUID}`,
> `/items/{itemUUID}`). The int64 PK stays internal-only.

## Slice-by-Slice Design

### `client` (was `participant`)

- `type` enum: `ndis | standard`.
- **standard** client requires: `name`. Optional: contact fields, `reference`,
  `payer_id`.
- **ndis** client additionally surfaces/requires: `plan_start`, `plan_end`,
  `mgmt_type`, and (when `mgmt_type='plan'`) `payer_id` (the plan manager).
- All NDIS-only columns (`plan_start`, `plan_end`, `mgmt_type`) are **nullable**.
  Required-field enforcement is gated on `type='ndis'` at the service boundary.
- `reference` is a free generic string (NDIS providers store the NDIS number
  there; generic providers store a customer code or leave it blank).

### `payer` (was `plan_manager`)

- Same entity shape (tenant-scoped contact, snapshotted onto invoice/estimate as
  `payer_snapshot`). Renamed only. Optional for any client, not just NDIS.

### `pricelist` / `items` (was `catalog` / `support_items`)

- `items`: `code` (optional), `name` (required), `unit`, `category` (optional
  text — replaces `support_category` + `registration_group` + `claim_type`),
  `taxable` (default true), plus the existing `item_prices` rows.
- `item_prices`: keep `zone` + `price_cap` columns, **nullable**. Only NDIS
  price-guide rows carry zone caps; generic items have a single base price and no
  cap.
- **Versioning kept.** A generic tenant gets one auto-created "Default price
  list" version on first item creation; an NDIS tenant keeps dated versions. A
  line still pins `price_list_version_id` when it references a catalogue item.

### `LineValidator` (in `internal/billing`) → conditional

Steps fire on data, not mode. Refactor the existing 6-step validator so each
NDIS-specific step short-circuits when its data is absent:

1. Catalogue lookup + snapshot (code, name) — **only if** the line references an
   `item_id`. Free-form/custom lines skip this entirely.
2. Price-cap assertion (`unit_price ≤ cap`) — **only if** the resolved item row
   carries a non-null cap. Generic items → no cap → skip.
3. Plan-window assertion (`service_date ∈ [plan_start, plan_end]`) — **only if**
   the client has both plan dates set.
4. Tax: driven by the per-line `taxable` flag. Default `true` for generic lines;
   catalogue items default the flag from the item's `taxable` value. No
   NDIS-authoritative override.
5. Non-negativity (`quantity ≥ 0`, `unit_price ≥ 0`) — always.

`ValidationResult` / `FieldError` / `ValidationError` shapes unchanged.

### `session` (was `shift`)

- Generic billable session / work-log: `service_date`, `note`, `tags`, status
  lifecycle, owns line-items, links to an invoice once billed.
- Rename status labels to generic terms (e.g. `recorded → delivered`,
  `drafted → billed`; keep `sent`, `paid`). Lifecycle structure unchanged.
- Optional for everyone — an invoice can be created directly from items/custom
  lines without a session.

### `business_profile`

- `zone` kept but nullable, NDIS-only. Surfaced in settings only when relevant.

### Invoice / Estimate / Recurring

- No structural change beyond the field renames (`item_id`,
  `price_list_version_id`, `taxable`). The dual-mode line (catalogue item OR
  custom item OR free-form) already exists and is the generalization hook.

## Generic Catalogue Import (replaces fixed NDIS XLSX ingest)

Reuses the already-generic `importer.ParseRows` (CSV **and** XLSX, configurable
header row). Deletes the NDIS-format-specific `catalog.ParseXLSX` and its
hardcoded column constants. Two-step flow, no new dependencies:

### Step 1 — Inspect
`POST …/items/import/inspect` (multipart: `file`, optional `headerRow`,
`sheetName`). Server runs `ParseRows`, returns `{ headers: string[],
sampleRows: object[] }` (first ~10 rows). **Nothing persisted.**

### Step 2 — Commit
`POST …/items/import/commit` (multipart: `file`, `mapping`, `label`, optional
`effectiveFrom`, `headerRow`, `sheetName`). `mapping` is `{ sourceHeader →
targetField }` where target fields are the generic item fields:

- `name` (required), `code`, `unit`, `unitPrice`, `category`, `taxable`.

Server applies the mapping, builds generic `Item` values, and inserts **one new
`price_list_version` + its items atomically** (whole-file-or-nothing; keep the
existing reject-on-error-no-partial-state behaviour). Broadcasts an SSE event
after commit (existing pattern).

Both endpoints gated `owner`/`admin` (existing role gate).

### Frontend
Upload file → choose header row → one dropdown per detected source column
selecting its target field (or "ignore") → preview the mapped sample → commit.
Replaces the current XLSX-only, no-mapping upload form.

### Deliberate simplifications (`ponytail:` markers in code)
- `// ponytail: single price column on import; multi-zone NDIS cap import later`
- One-shot mapping per upload; no saved templates yet.
- No diff preview — versioning protects prior data.

## Data Model Impact

- Rewrite the affected tenant migrations (`internal/db/migrations/tenant/`) with
  the renamed tables/columns. Regenerate `internal/db/gen` via `sqlc generate`
  after updating `internal/db/queries/*.sql`.
- Update both ERDs per CLAUDE.md: `docs/data-model.md` and the DB-per-tenant
  spec's Mermaid ERD.
- Catalogue tables remain **tenant-owned** (in the tenant DB), per the current
  architecture.

## Testing

- Go: `go test ./... -race`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./cmd/tallyo`.
- New/updated unit tests:
  - `LineValidator`: a generic line (no `item_id`, no cap, no plan window) passes
    with only tax + non-negativity; an NDIS line still enforces cap + plan window.
  - Import: `inspect` returns headers + sample without persisting; `commit`
    applies a mapping and creates a version; a missing required `name` mapping is
    rejected; partial-failure rolls back the whole upload.
- Frontend: `npm run check` (0 errors / 0 warnings), import-mapping component.

## Risks / Open Items

- **Rename churn** is large (touches most slices, queries, gen, frontend types
  and routes). Mitigated by clean-break (no data migration) and by doing it as a
  sequenced plan, slice by slice, keeping the build green at each step.
- NDIS zone-cap import remains manual until the deferred multi-zone import lands.
