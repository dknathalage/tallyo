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
- Partial unique `(logical_id) WHERE is_current = 1` — at most one current
  version per item.

### Line-item references

Both `line_items` and `estimate_line_items` today carry **two** reference
paths to the catalogue. Collapse them:

- **Drop** `custom_item_id`, `item_id`, `price_list_version_id`.
- **Add** `catalogue_item_id` TEXT NULL, FK → `catalogue_items.id`,
  `ON DELETE SET NULL`.

A line references the exact **version row** it priced from. Free-text lines
leave it NULL. Copy-on-write guarantees a referenced version row is never
mutated or hard-deleted, so `ON DELETE SET NULL` is only a backstop.

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

## Billing layer (`internal/billing`)

Rename, no behaviour change:
- `ResolveCustomItemID` → `ResolveCatalogueItemID`.
- `LineItem.CustomItemID` / `CustomItemUUID` → `CatalogueItemID` /
  `CatalogueItemUUID`.
- `isSupportItemLine` → `isCatalogueLine`.
- Catalogue lines price from `catalogue_items.unit_price`. The `LineValidator`
  is unchanged (per-line `taxable` × tenant default rate; non-negativity).

The shared billing mechanics (`NextNumber`, `InsertLineItems`,
`SnapshotBuilder`) keep storing the frozen description/quantity/rate on the
line; `catalogue_item_id` is provenance only.

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
  (replace `CustomItem` / `CustomItemInput`).
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
