# Catalogue merge — design

**Date:** 2026-06-25
**Status:** Approved (brainstorm) — pending implementation plan

## Summary

Merge the two overlapping "item" concepts — the `customitem` slice (flat,
user-CRUD line templates) and the `pricelist` slice (versioned, bulk-imported
price list) — into a **single per-tenant catalogue**. The catalogue is the one
source of reusable priced line templates. Versioning moves from
release-snapshot (one `price_list_versions` row per import) to **per-item,
copy-on-write**: an item only forks a new version when an edit would change a
version already referenced by an invoice/estimate line.

This is a rename **and** a structural merge:
- `internal/customitem/` and `internal/pricelist/` are deleted.
- A new `internal/catalogue/` slice replaces both.
- Schema is a clean break — old tables dropped, no data carried forward.

## Motivation

The catalogue is custom per tenant; "custom items" and "price list" were two
names for what users experience as one thing — their list of priced items. The
price-list release-versioning model added a multi-version lifecycle that the
flat custom-items list lacked, and invoice lines reached the catalogue through
two different reference paths. Collapsing to one concept removes the duplicate
slice, the dual reference paths, and the user-facing ambiguity.

## Decisions (from brainstorm)

| Question | Decision |
|---|---|
| Scope | Full rename including DB schema |
| Collision (two "catalogues") | Merge the two concepts into one |
| Lifecycle | Per-item versioning (not release-snapshot) |
| Storage | One append-only table, `logical_id` + `is_current` + `version` |
| Version trigger | Copy-on-write — mutate in place until a version is referenced, then fork |
| Existing data | Clean break — drop `custom_items`, `price_list_versions`, `items` |
| Import | Keep the upload-and-map import, folded into the catalogue slice |
| Version-history UI/endpoint | Out of scope (YAGNI) |

## Data model

### `catalogue_items` (tenant goose sequence)

A single append-only table. Each row **is a version** of a catalogue item;
rows that share a `logical_id` are the version history of one item.

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK (uuid) | The version row id. Invoice/estimate line items FK to this. |
| `logical_id` | TEXT (uuid) | Stable identity across an item's versions. |
| `tenant_id` | TEXT | Tenancy guard; not an FK (validated in app). |
| `code` | TEXT NULL | Optional catalogue code; the upsert key for import. |
| `name` | TEXT | Required. |
| `unit` | TEXT | |
| `category` | TEXT NULL | |
| `unit_price` | REAL | Base per-unit price. |
| `taxable` | INTEGER | bool. |
| `metadata` | TEXT | |
| `version` | INTEGER | 1, 2, 3, … per `logical_id`. |
| `is_current` | INTEGER | bool; exactly one current row per live `logical_id`. |
| `created_at` | TEXT | |
| `updated_at` | TEXT | |

Indexes:
- `(tenant_id, is_current)` — list the current catalogue.
- `(logical_id)` — fetch an item's version history.
- Partial unique `(logical_id) WHERE is_current = 1` — **at most one** current
  version per item. This permits the Delete-tombstone case where *all* rows of a
  `logical_id` are `is_current = 0`; do not add a "must have exactly one current"
  constraint, which would break Delete.

### Line-item references

Both `line_items` and `estimate_line_items` today carry **three** reference
columns to the catalogue: `item_id` (price-list item uuid), `custom_item_id`
(FK to `custom_items`), and `price_list_version_id` (pinned release). Collapse
all three into one:

- **Drop** `item_id`, `custom_item_id`, `price_list_version_id`.
- **Add** `catalogue_item_id` TEXT NULL, FK → `catalogue_items.id`,
  `ON DELETE SET NULL`.

A line references the exact **version row** it priced from. Free-text lines
leave it NULL. Copy-on-write guarantees a referenced version row is never
mutated or hard-deleted, so `ON DELETE SET NULL` is only a backstop.

#### `billing` public line-item fields collapse too

`billing.LineItem` / `LineItemInput` currently expose three distinct public
JSON fields mirroring those columns: `itemId` (`ItemID`), `customItemId`
(`CustomItemUUID`), and `priceListVersionId` (`PriceListVersionID`). These
collapse into a **single** field:

- **Remove** `itemId`, `customItemId`, `priceListVersionId`.
- **Add** `catalogueItemId` (`CatalogueItemID *string`) — the catalogue
  version-row uuid the line priced from, or null for a free-text line.

All consumers of the old three fields — estimate `convert.go` / `mapper.go` /
`repository.go`, the invoice/recurring snapshot/insert paths, and the SPA
`LineItemsEditor` + types — move to the one `catalogueItemId`. This is the
breaking API change the merge implies; there is no compatibility shim.

## Slice behaviour (`internal/catalogue/`)

Standard slice anatomy (`handler.go` / `service.go` / `repository.go` /
`types.go` / `query.go`), plus the import (`import_service.go` or folded into
`service.go`, mirroring how `pricelist` split read vs import).

Types:

```go
type CatalogueItem struct {
    ID        string // version row uuid
    LogicalID string
    Code      string // "" if unset
    Name      string
    Unit      string
    Category  string // "" if unset
    UnitPrice float64
    Taxable   bool
    Metadata  string
    Version   int
    IsCurrent bool
    CreatedAt string
    UpdatedAt string
}

type CatalogueItemInput struct {
    Code      string
    Name      string // required
    Unit      string
    Category  string
    UnitPrice float64
    Taxable   bool
    Metadata  string
}
```

### Operations

- **List** — `WHERE tenant_id = ? AND is_current = 1`, ordered/filtered like the
  old custom-items DataTable (filter columns: `name`, `unit_price`, `unit`,
  `category`, `taxable`).
- **Search** — current rows, all-fields match (used by the line-item picker and
  by `smarts`).
- **Get** — by `id` (a specific version) → 404 if absent.
- **Create** — new `logical_id`, `version = 1`, `is_current = 1`.
- **Update(id, input)**:
  1. Resolve the current row for the item.
  2. Check whether that row's `id` is referenced by any
     `line_items.catalogue_item_id` or `estimate_line_items.catalogue_item_id`.
  3. **Referenced** → INSERT a new row (same `logical_id`, `version + 1`,
     `is_current = 1`) and flip the old row to `is_current = 0`. The old row
     stays, frozen, still referenced.
  4. **Not referenced** → UPDATE the current row in place.
- **Delete** — flip every row of the `logical_id` to `is_current = 0` (a
  tombstone). Referenced versions linger so existing documents stay intact.
  `// ponytail:` no physical cleanup of unreferenced tombstoned rows — add a
  sweep only if table growth ever matters.
- **BulkDelete** — same, for a set of `logical_id`s; 400 on an unknown id.

Every mutation goes through `audit.WithTx` and broadcasts an SSE event via the
slice's `events.Notifier` with entity `"catalogue_item"`. Validation
(`name` required) lives in `CatalogueItemInput.Validate()` in the service.

### Reference check

A small repo method `IsVersionReferenced(ctx, versionID) (bool, error)` runs
two `EXISTS` queries against `line_items` and `estimate_line_items`. This is the
only cross-table read the slice needs and goes through the central `db/gen`
(no slice-to-slice import).

**Recurring templates are deliberately NOT in this check.** `recurring_templates`
store frozen line snapshots in JSON and carry **no** catalogue reference at all
(see the Recurring section) — they bill the price frozen in the template and
never run the validator. So a template never pins a version row, and the
reference check stays two `EXISTS` queries over the two line-item tables.

## Import (folded in)

The `pricelist` upload-and-map flow moves into the catalogue slice unchanged in
spirit:

- `POST …/catalogue/import/inspect` — owner/admin; returns detected headers +
  row sample (`importer` inspect).
- `POST …/catalogue/import/commit` — owner/admin; `importer.ApplyMapping`
  produces typed rows, then each row **upserts by `code`**:
  - Known `code` → treat as an update (copy-on-write rules apply).
  - New `code` (or no code) → create.

`importer.ApplyMapping` is reused as-is.

Note the copy-on-write consequence for bulk re-import: a re-imported `code`
whose current version is **unreferenced** updates in place; one whose current
version is **referenced** by an existing document forks a new version. So a
large re-import is a mix of in-place updates and forks — old documents keep
their pinned versions either way.

## Billing layer (`internal/billing`) — validator REDESIGN

This is **not** a rename. `LineValidator` (`internal/billing/validation.go`)
today implements the *release-snapshot* pricing pipeline the merge removes:
`resolveVersion` resolves a price-list version **by service date**
(`ResolveVersionForDate`) or honours a pinned `PriceListVersionID`;
`validateSupportLine` looks up the item **by `code` within that version**
(`GetItemByCode`); `snapshotSupportItem` pins `item_id` + `price_list_version_id`.
Per-item copy-on-write has no per-release versions and no version-by-date
resolution, so that whole path is replaced.

### New catalogue-line validation

A line is a **catalogue line** iff it carries `catalogueItemId`
(`isCatalogueLine` replaces `isSupportItemLine` — the new test is "has a
catalogue item id", not "has a code and no custom-item id"). For such a line:

1. Fetch the referenced catalogue row directly by id:
   `catalogue.Repo.GetByID(ctx, tenantID, catalogueItemId)` → 422 field error
   if absent. The line already names the exact version row, so there is **no
   version-by-date step and no `code` lookup**. Re-validating an existing
   document re-reads the same frozen row (copy-on-write guarantees it is
   unchanged), so prices never drift — replacing the old "honour the pinned
   version" branch.
2. Snapshot from the row: fill `code`, fill `description` from `name` when the
   caller left it blank, set `taxable` from the row (catalogue is authoritative).
3. Fill `unitPrice` from the row's `unit_price` when the caller supplied none
   (`applyItemUnitPrice`, unchanged in spirit — now reads `catalogue_items`).

The **service-date-required rule for catalogue lines is dropped** — pricing no
longer needs it. `serviceDate` stays as optional line metadata.

A free-text line (no `catalogueItemId`) keeps the existing non-negativity-only
path with caller-controlled price/taxable.

### Dependency swap

`LineValidator.cat` changes from `*pricelist.ItemsRepo` to the catalogue repo
(an interface the validator declares: `GetByID(ctx, tenantID, id) (*CatalogueItem, error)`),
wired in `internal/app`. Removed from the validator's surface:
`ResolveVersionForDate`, `GetVersionByUUID`, `GetItemByCode`, `resolveVersion`,
the version label/UUID plumbing.

Unchanged: the tax contract (`computeLineTax` — taxable lines ×
tenant-default rate, `Round2`), `defaultTaxRate`, `ValidationError`/`FieldError`,
the `Validate` / `ValidateFilling` signatures (callers untouched).

`ResolveCustomItemID` → `ResolveCatalogueItemID` in `lineitem.go`; the shared
mechanics (`NextNumber`, `InsertLineItems`, `SnapshotBuilder`) keep storing the
frozen description/quantity/rate on the line — `catalogue_item_id` is provenance.

## Recurring templates (`internal/recurring`)

**Current behaviour (verified):** `recurring_templates.line_items` is a JSON
line template. `generate.go` (`parseLines` → `generateTx` → `InsertLineItems`)
bills the **price frozen in the template JSON** (`RecurringLine.UnitPrice`); it
does **not** run the `LineValidator` and does **not** re-resolve against the
catalogue (`service.go` documents this as the J10/J11 deferral). The merge keeps
that behaviour — recurring stays frozen-price, the validator stays deferred.

So recurring needs **no catalogue link at all**. The only change is to drop the
two now-dead reference fields from `RecurringLine`:

- **Remove** `ItemID` (`itemId`) and `CustomItemID` (`customItemId`) from the
  `RecurringLine` JSON shape. The template line keeps its frozen snapshot
  fields (`description`, `quantity`, `unitPrice`, `unit`, `taxable`, `code`).
- Generated invoice/estimate lines therefore have `catalogue_item_id = NULL`
  (frozen snapshots, no provenance link) — consistent with the rest of the
  clean break, where existing documents keep their frozen line data and lose
  only the catalogue provenance link.
- Clean break: pre-existing `recurring_templates.line_items` JSON carrying
  `itemId` / `customItemId` keys is **abandoned** — the new `parseLines` ignores
  unknown keys; templates re-saved through the new picker repopulate cleanly.

Consequence: templates never reference a catalogue version row, so they are
correctly absent from `IsVersionReferenced`, and editing a catalogue item in
place can never corrupt a template (the template carries its own frozen price).
A future "recurring tracks current catalogue prices" feature is explicitly out
of scope — it would un-defer J10/J11 and is not part of this merge.

## HTTP routes

New, under the tenant-scoped authenticated group `/api/t/{tenantUUID}/`:

| Method | Path | Gate |
|---|---|---|
| GET | `/catalogue` | auth |
| POST | `/catalogue` | auth |
| GET | `/catalogue/{uuid}` | auth |
| PUT | `/catalogue/{uuid}` | auth |
| DELETE | `/catalogue/{uuid}` | auth |
| POST | `/catalogue/bulk-delete` | auth |
| POST | `/catalogue/import/inspect` | owner/admin |
| POST | `/catalogue/import/commit` | owner/admin |

Removed: all `/custom-items*` and `/price-list/*` routes. `// ponytail:` no
`/catalogue/{uuid}/versions` history endpoint until a UI needs it.

## Frontend (`web/`)

- Routes: add `/[tenant]/catalogue/+page.svelte` (list, DataTable) and
  `/[tenant]/catalogue/[uuid]/+page.svelte` (detail/edit). Delete
  `/[tenant]/custom-items/**` and any price-list pages.
- Store: `src/lib/stores/catalogue.svelte.ts` — `createCollectionStore` with
  endpoint `'catalogue'`, event channel `'catalogue_item'`. Delete
  `customItems.svelte.ts`.
- Types: `CatalogueItem` / `CatalogueItemInput` in `src/lib/api/types.ts`
  (replace `CustomItem` / `CustomItemInput`). Note the field remap from the old
  custom-item shape: `rate` → `unitPrice`, and the catalogue adds `code` +
  `category` (the flat custom item had neither). The detail/edit form gains
  those two optional fields.
- Components: `LineItemsEditor.svelte` picker label "From catalogue", searches
  `catalogue`; sidebar nav label "Catalogue" in `+layout.svelte`.

## Smarts (`internal/smarts`)

Repoint the catalogue `search` tool and the `map-price-list-import` Smart from
`pricelist` to the new catalogue slice. The search stays a tenant-scoped,
all-fields match over current catalogue rows. No agent-loop or schema changes.

## Migration (clean break)

One new goose migration in `internal/db/migrations/tenant/`:
1. `DROP TABLE custom_items;`
2. `DROP TABLE items;` then `DROP TABLE price_list_versions;`
3. `CREATE TABLE catalogue_items (…)` + indexes above.
4. Rewrite `line_items` and `estimate_line_items`: drop the three old columns,
   add `catalogue_item_id` (FK, `ON DELETE SET NULL`). (SQLite: rebuild the
   table — create new, copy carried columns, drop old, rename — since SQLite
   can't drop an FK column in place cleanly.)

No data is carried forward. Existing invoices keep their frozen line-item
snapshots (description/quantity/rate already stored on the line); they lose only
the catalogue provenance link.

Then `sqlc generate` to regenerate `internal/db/gen` from the new
`queries/catalogue.sql` (replacing `custom_items.sql` and the price-list
queries) and the rewritten `line_items.sql` / `estimate_line_items.sql` joins.

## Docs to update

- `CLAUDE.md` — slice list (drop `customitem`, `pricelist`; add `catalogue`);
  the "Price list" section becomes the "Catalogue" section.
- `docs/data-model.md` — ERD: drop `custom_items`, `items`,
  `price_list_versions`; add `catalogue_items`; update line-item FKs.

## Out of scope / YAGNI

- Version-history UI and `/catalogue/{uuid}/versions` endpoint.
- Physical cleanup of tombstoned/unreferenced version rows.
- Any data migration from the old tables.
- Re-introducing pricing zones, price caps, or plan windows (never existed; not
  added here).

## Testing

- `catalogue` slice: service tests for create / update-in-place /
  update-forks-when-referenced / delete-tombstones / bulk-delete / search;
  repository tests for `IsVersionReferenced` and the copy-on-write fork;
  handler tests for the route surface and the owner/admin import gate.
- `internal/app` integration: catalogue CRUD over HTTP, import inspect/commit,
  and an invoice line referencing a catalogue version that then forks on the
  next catalogue edit (old invoice keeps its pinned version).
- `billing`: existing line-item tests updated for the renamed
  `CatalogueItem*` fields.
- Frontend: `svelte-check` clean; the catalogue route + store compile.
