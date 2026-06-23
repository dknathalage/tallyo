# Remove NDIS — Pure Generic Invoicer

**Date:** 2026-06-23
**Status:** Design approved, pending spec review
**Branch:** feat/generic-invoicing-de-ndis

## Problem

The earlier de-NDIS refactor renamed NDIS entities to generic names and made
NDIS behaviour "optional", but NDIS is still woven into the **default** UX and
schema: every tenant has an NDIS pricing zone in Settings, every client carries
an NDIS `type` + plan window + management type, and the line validator enforces
NDIS zone price-caps and plan windows. The product is meant to be a **generic
goods-and-services invoicer**. NDIS leaks everywhere.

## Decision

**Remove NDIS entirely.** The app becomes a pure generic invoicer. Two fields
that originated in NDIS but are generically useful are **kept** (renamed already,
no NDIS wording): the client `payer` (an optional third party billed on behalf
of the client) and the client `reference` (a free-text external/customer code).

This reverses the earlier "NDIS must still work" position — the user has chosen
a clean generic app over retaining NDIS.

## Goal

A tenant signs up (no NDIS prompt), adds clients (just name + contact +
optional reference + optional payer), builds a price list (items with a
`unit_price`), and invoices — from catalogue items, custom items, or free-form
lines. No zones, no price caps, no plan windows, no client types, no NDIS
wording anywhere.

## Non-Goals

- No data migration (clean-break, consistent with the project). Existing dev
  data is recreated fresh.
- No feature flag / toggle — NDIS is gone, not gated.
- Keeping NDIS price-cap or plan-window enforcement in any form.

## Scope — Remove

### Schema (`internal/db/migrations/tenant/00001_tenant.sql`, `00003_catalogue.sql`)
- `business_profile`: drop the `zone` column.
- Drop the `item_prices` table entirely (zone-based price caps).
- `clients`: drop `type`, `plan_start`, `plan_end`, `mgmt_type`. **Keep**
  `reference`, `payer_id` (FK to `payers`), name, email, phone, address,
  metadata.
- `items`: unchanged (`code`, `name`, `unit`, `category`, `unit_price`,
  `taxable`, `metadata`). `price_list_versions` unchanged.
- `line_items` / `estimate_line_items`: unchanged (`item_id`,
  `price_list_version_id`, `code`, `service_date`, `description`, `unit`,
  `quantity`, `unit_price`, `taxable`, …). `service_date` stays (a generic
  line date used for version resolution).
- Regenerate `internal/db/gen` (drop `item_prices` model/queries; `clients`
  loses the four columns).

### Backend

**`internal/billing/validation.go` — collapse the validator.**
For a catalogue line (carries a `code`, not a custom item):
1. Resolve the price-list version whose window contains `service_date`.
2. Find the item by code in that version; snapshot `code`, `name`/description,
   pin `price_list_version_id`, set `taxable` from the item.
3. Price: if the caller supplied `unit_price ≤ 0` and the item has a
   `unit_price`, fill it from the item; otherwise keep the caller's price.
4. Non-negativity (`quantity ≥ 0`, `unit_price ≥ 0`).
Custom / free-form lines: non-negativity only (unchanged). Tax: unchanged
(per-line `taxable` × tenant default rate).
- **Remove:** `tenantZone`, `applyZonePrice`, the zone-cap assertion, the
  fill-mode zone-cap overwrite, `assertPlanWindow`, and the `control` parameter
  of `NewLineValidator` (now wholly unused — the catalogue is tenant-owned).
- `ValidateFilling` (agent path): keep the method, but "filling" now means fill
  from `items.unit_price` (same as the default path's price-fill) rather than a
  zone cap. Effectively `Validate` and `ValidateFilling` converge; keep one
  code path and have the agent call it. (Decide during implementation whether to
  keep `ValidateFilling` as a thin alias or drop it and update the agent caller.)
- `profiles` dependency: the validator no longer reads the zone, so it no longer
  needs `businessprofile`. Drop that field from `LineValidator`.

**`internal/pricelist`**
- Drop the `ItemPrice` type, `item_prices` repo methods, `ResolveZonePrice`,
  the `item_prices` queries, and the `…/items/{itemUUID}/prices` read endpoint +
  its frontend usage.
- Keep items, versions, the import (`Inspect`/`ImportMapped`/`ApplyMapping`).

**`internal/client`**
- Drop `type`, `plan_start`, `plan_end`, `mgmt_type` from `Client`/`ClientInput`,
  the repo create/update, and the queries. Drop `validateClientInput`'s NDIS
  gating + `errInvalidType` (a client now needs only a name). Keep `reference`,
  `payer` resolution.

**`internal/businessprofile`**
- Drop `zone` from the type, repo, queries; drop `validZone`.

**`internal/app` (signup/auth)**
- Drop `zone` from `signupRequest`, `validateSignup`, `allowedZones`, and
  `auth.SignupInput` / `ProvisionBusinessProfile`.

**`internal/agent`**
- The divide/create-invoice path used catalogue-authoritative zone pricing via
  `ValidateFilling`; it now fills from `items.unit_price`. Update tool
  descriptions/prompts accordingly (no "zone"/"price cap").
- `CatalogueSearcher` interface (`deps.go`): drop the `zone` param from
  `SearchForDate`; `catalogueMatchView` drops `PriceCap`; the `search_catalogue`
  tool result/schema drops `priceCap`.

**Additional zone/cap surfaces found in spec review (must also be removed):**
- `pricelist.service.go`: the `Match` struct carries `Zone`/`PriceCap`/`Quotable`
  and `SearchForDate(ctx, query, serviceDate, zone, limit)` calls
  `ResolveZonePrice` per result — strip the zone fields + the `zone` param.
- `pricelist.repository.go`: the import write path calls `UpsertItemPrice` —
  remove it (generic import writes `unit_price` only).
- `client/repository.go`: the `ClientCols` listquery allowlist entries
  (`mgmtType`/`planStart`/`planEnd`/`type`), `normType`, and the `row.Scan`
  column order — compiler-invisible, fix by hand.
- Frontend: `LineItemsEditor.svelte` (per-line cap cache + "Cap (zone)" display +
  `loadPrices`), `businessProfile.svelte.ts` store (`zone` field + `'national'`
  default), the `priceList.loadPrices` API client method, and `zoneLabel` +
  prices column on the price-list page.

### Frontend (`web/`)
- Settings: remove the pricing-zone field entirely (signup field already
  removed).
- `ClientEditor.svelte`: remove the type `<select>`, plan-start/plan-end,
  management-type, and the `{#if clientType === 'ndis'}` gating. Show name,
  contact, optional reference, optional payer for every client.
- `clients/+page.svelte`: drop the `type` column + filter.
- `types.ts`: remove `ClientType` and `Zone`; drop `type`/`planStart`/`planEnd`/
  `mgmtType` from `Client`/`ClientInput`; drop the prices type if present.
- Remove the prices view from the price-list page.
- Remove any remaining NDIS strings.

## Components & Boundaries

- **Validator** (`billing`): now depends only on `pricelist` (catalogue),
  `client` (existence — actually only needs the client id is valid; may drop the
  `clients` dep too if the plan-window removal eliminates the only read) and
  `taxrate`. Confirm during implementation which deps remain; remove dead ones.
- **Pricelist**: items + versions + import only. No price/zone concept.
- **Client**: name + contact + reference + payer. No NDIS attributes.
- Cross-slice interfaces (`invoice.SessionLinker`, `session.InvoiceChecker`,
  payer resolution) unchanged.

## Data Flow

Invoice create → resolve client/payer uuids → validate lines (catalogue lookup +
unit_price fill + tax + non-negativity) → persist → SSE. No zone/cap/plan-window
reads.

## Error Handling

Validator still returns `*ValidationError` with field-level detail; the NDIS
messages ("exceeds price cap", "no price catalogue in effect for service date",
plan-window) are removed. Remaining catalogue errors: unknown code, no price
list in effect for the date, negative quantity/price.

## Testing

- Remove NDIS-specific tests across `billing` (`TestValidateOverCapRejected`,
  `…AtCap…`, `…Quotable…`, `…Zone…`, `TestValidateFilling*` cap behaviour,
  `assertPlanWindow` tests), `client` (`TestClientTypeFieldGating`),
  `businessprofile` (zone tests), `app` (signup zone, validation e2e cap/plan
  cases), `agent` (zone-cap fill fixtures).
- Keep/extend generic tests: catalogue line prices from `items.unit_price`
  (`TestGenericCodedLinePricesFromItemUnitPrice` → now the *only* coded-line
  pricing path), custom/free-form line, import inspect/commit, invoice/estimate
  create + totals + tax, the tenant-catalogue regression
  (`TestLineValidatorReadsCatalogueFromTenant`).
- Migration tests: assert `item_prices` absent, `clients` lacks the four
  columns, `business_profile` lacks `zone`.
- Full gate: `gofmt`/`vet` clean, `go test ./... -race`,
  `CGO_ENABLED=0 go build ./cmd/tallyo`, `npm run check` 0/0 + build.
- End-to-end smoke (real server, fresh data dir): signup (no NDIS prompt) →
  create a client (name only) → import a CSV price list → invoice a catalogue
  item by code (prices from unit_price) + a free-form line → 201, correct totals.

## Risks

- **Breadth:** touches billing, pricelist, client, businessprofile, auth/signup,
  agent, gen, and the SPA. Mitigated by clean-break (no data migration) and a
  phased plan that keeps the build green per phase.
- **Validator simplification must not change tax or custom-line behaviour** —
  only the zone/cap/plan-window paths are removed. Guard with the retained tax +
  custom-line tests.
- **`gen` regeneration** after dropping `item_prices` + client columns — ensure
  no dangling references; compiler is the worklist.
