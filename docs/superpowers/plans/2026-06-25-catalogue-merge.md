# Catalogue Merge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge the `customitem` and `pricelist` slices into one per-tenant `catalogue` slice with per-item copy-on-write versioning and a clean-break schema.

**Architecture:** One append-only `catalogue_items` table (`logical_id` + `is_current` + `version`); editing a referenced version forks a new row, else mutates in place. Line items collapse three catalogue references into one `catalogue_item_id`. The billing `LineValidator` is redesigned to price from the referenced row directly (no version-by-date). Recurring stays frozen-price and just sheds two dead fields.

**Tech Stack:** Go 1.26 (chi, sqlc, goose, modernc/sqlite), SvelteKit SPA.

**Spec:** `docs/superpowers/specs/2026-06-25-catalogue-merge-design.md`

> **Coupling note for the executor.** The backend is **one compile unit**: Task 2 (sqlc regen) drops `line_items` columns that `billing`, `invoice`, `estimate`, `session`, `recurring`, and `smarts` all reference, so `go build ./...` will not pass until Tasks 2–9 are all done. Do Tasks 1–9 as a single uninterrupted backend push; run `CGO_ENABLED=0 go build ./...` only as the Task 9 gate (earlier builds are expected to fail). Commit per task regardless (the tree is on a feature branch). Frontend (Tasks 10–11) and docs (Task 12) are independent and verify on their own.

---

## File structure

**New:** `internal/catalogue/{types.go,repository.go,service.go,handler.go,query.go,import.go}` (+ `*_test.go`), `internal/db/queries/catalogue.sql`, `internal/db/migrations/tenant/00009_catalogue.sql`, `web/src/lib/stores/catalogue.svelte.ts`, `web/src/routes/[tenant]/catalogue/+page.svelte`, `web/src/routes/[tenant]/catalogue/[uuid]/+page.svelte`.

**Modified:** `internal/billing/{lineitem.go,validation.go}`, `internal/db/queries/{line_items.sql,estimate_line_items.sql}`, `internal/{invoice,estimate,session}/*`, `internal/recurring/{types.go,query.go}`, `internal/smarts/draft_invoice.go` (+ any other smarts referencing pricelist), `internal/app/{app.go,server.go}`, `web/src/lib/api/types.ts`, `web/src/lib/components/LineItemsEditor.svelte`, `web/src/routes/[tenant]/+layout.svelte`, `CLAUDE.md`, `docs/data-model.md`.

**Deleted:** `internal/customitem/`, `internal/pricelist/`, `internal/db/queries/custom_items.sql`, the price-list queries, `web/src/lib/stores/customItems.svelte.ts`, `web/src/routes/[tenant]/custom-items/`, any price-list SPA pages, `internal/app/custom_items_test.go` (replaced).

---

## Task 1: Migration — clean-break schema

**Files:** Create `internal/db/migrations/tenant/00009_catalogue.sql`

- [ ] **Step 1: Write the migration.** Goose up/down. Up:
  - `DROP TABLE custom_items;`
  - `DROP TABLE items;` then `DROP TABLE price_list_versions;`
  - Create `catalogue_items`:
    ```sql
    CREATE TABLE catalogue_items (
        id          TEXT PRIMARY KEY,
        logical_id  TEXT NOT NULL,
        tenant_id   TEXT NOT NULL,
        code        TEXT,
        name        TEXT NOT NULL,
        unit        TEXT,
        category    TEXT,
        unit_price  REAL NOT NULL DEFAULT 0,
        taxable     INTEGER NOT NULL DEFAULT 0,
        metadata    TEXT NOT NULL DEFAULT '{}',
        version     INTEGER NOT NULL DEFAULT 1,
        is_current  INTEGER NOT NULL DEFAULT 1,
        created_at  TEXT NOT NULL,
        updated_at  TEXT NOT NULL
    );
    CREATE INDEX idx_catalogue_items_current ON catalogue_items (tenant_id, is_current);
    CREATE INDEX idx_catalogue_items_logical ON catalogue_items (logical_id);
    CREATE UNIQUE INDEX idx_catalogue_items_one_current ON catalogue_items (logical_id) WHERE is_current = 1;
    ```
  - Rebuild `line_items` and `estimate_line_items` to drop `item_id`, `custom_item_id`, `price_list_version_id` and add `catalogue_item_id TEXT REFERENCES catalogue_items(id) ON DELETE SET NULL`. SQLite can't drop an FK column cleanly, so use the create-new / copy-carried-columns / drop / rename dance, preserving all other columns and indexes from `00001_tenant.sql` (lines 130–154, 181–199). Since this is clean-break, the copy can omit the three dropped columns; `catalogue_item_id` starts NULL.
  - Down: reverse (best-effort; clean-break means down is rarely used — recreate old tables empty).
- [ ] **Step 2: Verify goose parses.** Run `go test ./internal/db/... -run Migrate` (or the app startup test). Expected: migrations apply without error.
- [ ] **Step 3: Commit.** `git add internal/db/migrations/tenant/00009_catalogue.sql && git commit -m "feat(db): catalogue_items table + line-item FK collapse migration"`

## Task 2: sqlc queries + regen

**Files:** Create `internal/db/queries/catalogue.sql`; delete `internal/db/queries/custom_items.sql` and the price-list query file(s); modify `internal/db/queries/line_items.sql`, `internal/db/queries/estimate_line_items.sql`.

- [ ] **Step 1: Write `catalogue.sql`.** Queries (sqlc):
  - `ListCatalogue` — `SELECT * FROM catalogue_items WHERE tenant_id = ? AND is_current = 1 ORDER BY name`
  - `SearchCatalogue` — current rows, all-fields LIKE (code/name/unit/category), `ORDER BY name`
  - `GetCatalogueItem :one` — by `tenant_id` + `id` (a specific version row)
  - `GetCurrentByLogical :one` — `WHERE tenant_id=? AND logical_id=? AND is_current=1`
  - `MaxVersionForLogical :one` — `SELECT COALESCE(MAX(version),0) ...`
  - `CreateCatalogueItem :one` — insert (id, logical_id, tenant_id, code, name, unit, category, unit_price, taxable, metadata, version, is_current, created_at, updated_at) RETURNING *
  - `UpdateCatalogueItemInPlace :one` — update fields + updated_at WHERE tenant_id+id RETURNING *
  - `MarkVersionStale :exec` — `UPDATE catalogue_items SET is_current=0 WHERE tenant_id=? AND id=?`
  - `TombstoneLogical :exec` — `UPDATE catalogue_items SET is_current=0 WHERE tenant_id=? AND logical_id=?`
  - `CatalogueVersionReferenced :one` — `SELECT EXISTS(SELECT 1 FROM line_items WHERE catalogue_item_id=? UNION ALL SELECT 1 FROM estimate_line_items WHERE catalogue_item_id=?)` (or two queries; keep it a single EXISTS over both)
  - `GetCatalogueIDByUUID :one` — `SELECT id FROM catalogue_items WHERE tenant_id=? AND id=? AND is_current=1` (for bulk-delete uuid validation → returns logical_id; adjust: bulk-delete operates on logical_id, so return logical_id)
- [ ] **Step 2: Rewrite `line_items.sql` + `estimate_line_items.sql`.** Replace the `LEFT JOIN custom_items ci ON li.custom_item_id = ci.id` + `ci.id AS custom_item_uuid` with `LEFT JOIN catalogue_items cat ON li.catalogue_item_id = cat.id` + `cat.id AS catalogue_item_uuid`. In `CreateLineItem` / `UpdateSessionLineItem*` and the estimate equivalents, replace the `item_id, custom_item_id, price_list_version_id` column set with single `catalogue_item_id`.
- [ ] **Step 2: Regenerate.** Run `"$(go env GOPATH)/bin/sqlc" generate`. Expected: `internal/db/gen` updates; `CustomItem`/`Item`/`PriceListVersion` gen types gone, `CatalogueItem` present, line-item rows now have `CatalogueItemID`/`CatalogueItemUuid`.
- [ ] **Step 3: Commit.** (build will not pass yet — expected.) `git add internal/db && git commit -m "feat(db): catalogue sqlc queries; line-item join collapse"`

## Task 3: catalogue slice

**Files:** Create `internal/catalogue/{types.go,repository.go,service.go,handler.go,query.go,import.go}`.

Copy the `customitem` slice shape (handler/service/repository/types) verbatim where it transfers — the slice is identical in structure. Deltas:

- [ ] **types.go** — `CatalogueItem` (fields per spec incl. `LogicalID`, `Version`, `IsCurrent`), `CatalogueItemInput` (`Code,Name,Unit,Category,UnitPrice,Taxable,Metadata`; `Validate()` requires `Name`). `CatalogueCols` listquery spec: `name`(Text), `unitPrice`(Number,col `unit_price`), `unit`(Text), `category`(Text), `taxable`(None).
- [ ] **repository.go** — `List`/`Query`/`Search`/`Get` (current rows only). `Create` (new logical_id, v1, current). `Update(tenantID, id, in)`:
  1. `GetCatalogueItem` by id → 404 if absent.
  2. `CatalogueVersionReferenced(id)`.
  3. referenced → `MaxVersionForLogical`+1, `CreateCatalogueItem` (same logical_id, is_current=1), then `MarkVersionStale(oldID)`.
  4. not referenced → `UpdateCatalogueItemInPlace`.
  All inside one `audit.WithTx`, entity `"catalogue_item"`. `Delete(tenantID, id)` → resolve logical_id, `TombstoneLogical`. `BulkDelete([]logicalID)`. `ResolveCatalogueLogicalIDs` (bulk-delete uuid→logical_id, unknown → error). `GetByID(ctx, tenantID, id) (*CatalogueItem, error)` for the validator (returns the exact version row, any is_current).
- [ ] **import.go** — fold the `pricelist` import: `Inspect(file) -> headers+sample` and `ImportMapped(mapping, rows)` that, per row, upserts **by code** through the Update/Create copy-on-write path (known current code for tenant → Update; else Create). Reuse `importer.ApplyMapping`.
- [ ] **service.go** — copy customitem service; entity `"catalogue_item"`; add `Inspect`/`ImportMapped` passthroughs and a `GetByID` exposed for app wiring into the validator.
- [ ] **handler.go** — routes: `GET/POST /catalogue`, `GET/PUT/DELETE /catalogue/{uuid}`, `POST /catalogue/bulk-delete`, `POST /catalogue/import/inspect`, `POST /catalogue/import/commit`. (import handlers ported from pricelist handler.)
- [ ] **Tests** — port `customitem` + `pricelist` tests; add: update-in-place (unreferenced), update-forks (referenced version stays frozen), delete-tombstones, search current-only, import upsert-by-code. Run `go test ./internal/catalogue/...`.
- [ ] **Commit.** `git add internal/catalogue internal/db/queries/catalogue.sql && git commit -m "feat(catalogue): per-item copy-on-write slice (replaces customitem+pricelist)"`

## Task 4: billing line-item field collapse

**Files:** Modify `internal/billing/lineitem.go`.

- [ ] Collapse `ItemID`/`CustomItemID`/`CustomItemUUID`/`PriceListVersionID` on `LineItem`, `LineItemInput`, `LineItemRow` into one `CatalogueItemID *string` (json `catalogueItemId`) on the API structs and `CatalogueItemID`/`CatalogueItemUuid` on `LineItemRow`. Update the four `LineItemRowFrom*` mappers to read `r.CatalogueItemID` + `r.CatalogueItemUuid`, and `LineItemFromRow`.
- [ ] Rename `ResolveCustomItemID` → `ResolveCatalogueItemID` using `GetCatalogueIDByUUID` (validates the uuid is a current catalogue row for the tenant); rename `ErrUnknownCustomItem` → `ErrUnknownCatalogueItem`.
- [ ] Commit (build still red).

## Task 5: billing validator redesign

**Files:** Modify `internal/billing/validation.go`.

- [ ] Replace the `cat *pricelist.ItemsRepo` dependency with `cat *catalogue.Repo` — **billing imports catalogue directly**, exactly as it imports pricelist today (verified: pricelist/customitem don't import billing, and catalogue won't either, so no cycle). `NewLineValidator(tenant db.Executor)` keeps its signature and builds `catalogue.NewRepo(tenant)` internally — no app-wired adapter, no new interface. `// ponytail:` direct import mirrors the existing billing→pricelist edge.
- [ ] Rewrite the line path: `isCatalogueLine(line)` = `line.CatalogueItemID != nil && *line.CatalogueItemID != ""`. For a catalogue line: `cat.GetByID(ctx, tenantID, *line.CatalogueItemID)` → 422 if absent; snapshot code/name(→description if blank)/taxable; fill unit_price when caller price ≤ 0. Delete `resolveVersion`, `validateSupportLine`'s version logic, the service-date-required rule, `GetItemByCode`/`ResolveVersionForDate`/`GetVersionByUUID` usage, `snapshotSupportItem`'s version pinning. Keep `computeLineTax`, `defaultTaxRate`, `applyItemUnitPrice` (now reads the catalogue row).
- [ ] `NewLineValidator` and `Validate`/`ValidateFilling` signatures unchanged — callers (invoice/estimate/recurring/session) untouched.
- [ ] Run `go test ./internal/billing/...` after Task 6 wiring (validator tests need the lookup stub). Commit.

## Task 6: invoice / estimate / session consumers

**Files:** Modify line-item create/update/insert paths in `internal/invoice`, `internal/estimate`, `internal/session`, and `internal/billing/lineitems.go` (`InsertLineItems`).

- [ ] Update every `CreateLineItemParams` / `UpdateSessionLineItem*Params` / estimate equivalents to pass single `CatalogueItemID` (resolved via `ResolveCatalogueItemID`) instead of the three old params. Update any mapper/convert (`estimate/convert.go`, `estimate/mapper.go`) referencing the dropped fields.
- [ ] `go build ./internal/...` for these packages; fix references. Commit.

## Task 7: recurring

**Files:** Modify `internal/recurring/types.go`, `internal/recurring/query.go`.

- [ ] Remove `ItemID` + `CustomItemID` from `RecurringLine`. In `parseLines` drop the `ItemID`/`CustomItemID` assignments (generated lines get `CatalogueItemID = nil`). `unmarshalLines` already ignores unknown keys (clean break: old JSON abandoned).
- [ ] Run `go test ./internal/recurring/...`. Commit.

## Task 8: smarts repoint

**Files:** Modify `internal/smarts/draft_invoice.go` (+ any other smarts file importing `pricelist`).

- [ ] Replace the `pricelist`-based grounding `search` (version-pinned `SearchItems`) with the catalogue `Search` (current rows, all-fields). Update the `map-price-list-import` Smart to target the catalogue import. Replace the `s.cat` dependency type. Drop the `ver.ID` version-resolution; search is over current catalogue.
- [ ] Run `go test ./internal/smarts/...`. Commit.

## Task 9: app wiring + delete old slices (BACKEND GATE)

**Files:** Modify `internal/app/app.go`, `internal/app/server.go`; delete `internal/customitem/`, `internal/pricelist/`, `internal/app/custom_items_test.go` (port needed cases to a new `catalogue_test.go`).

- [ ] Replace `customItemSvc`/`priceListSvc`/`priceListImportSvc` with one `catalogueSvc := catalogue.NewService(database, hub)`. `billing.NewLineValidator` is unchanged (builds its own catalogue repo). Replace `smarts.NewService(..., pricelist.NewItems(database), ...)` with the catalogue search dependency. `Deps.CustomItems`/`Deps.PriceList` → `Deps.Catalogue *catalogue.Handler`; route registration → `deps.Catalogue.Routes(pr)` (import gated owner/admin inside the slice's Routes via `httpx.RequireRole`, matching how pricelist gated import).
- [ ] `rm -rf internal/customitem internal/pricelist`.
- [ ] **GATE:** `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l . && go test ./... -race`. Expected: all clean.
- [ ] Commit. `git commit -m "feat(catalogue): wire catalogue slice, remove customitem+pricelist"`

## Task 10: frontend — types, store, routes

**Files:** Create `web/src/lib/stores/catalogue.svelte.ts`, `web/src/routes/[tenant]/catalogue/+page.svelte`, `web/src/routes/[tenant]/catalogue/[uuid]/+page.svelte`. Modify `web/src/lib/api/types.ts`. Delete `web/src/lib/stores/customItems.svelte.ts`, `web/src/routes/[tenant]/custom-items/`, price-list pages.

- [ ] `types.ts`: replace `CustomItem`/`CustomItemInput` with `CatalogueItem`/`CatalogueItemInput` (`code,name,unit,category,unitPrice,taxable,metadata` + read-only `version`). Update `LineItem`/`LineItemInput` to one `catalogueItemId` (drop `itemId`/`customItemId`/`priceListVersionId`).
- [ ] Store: copy `customItems.svelte.ts` → `catalogue.svelte.ts`, endpoint `'catalogue'`, event channel `'catalogue_item'`.
- [ ] Routes: copy the custom-items list + detail pages; rename fields (`rate`→`unitPrice`, add `code`+`category` inputs). Add an import-wizard entry point if the price-list pages had one (port it under `/catalogue`).
- [ ] Run `cd web && npm run check`. Commit.

## Task 11: frontend — editor + sidebar

**Files:** Modify `web/src/lib/components/LineItemsEditor.svelte`, `web/src/routes/[tenant]/+layout.svelte`.

- [ ] LineItemsEditor: "From catalogue" picker label; search `catalogue`; set `catalogueItemId` on the line.
- [ ] Sidebar: nav label "Catalogue", route `/catalogue` (replace the "Custom items" + any "Price list" entries).
- [ ] `npm run check` clean. Commit.

## Task 12: docs

**Files:** Modify `CLAUDE.md`, `docs/data-model.md`. Also trim the stale J10/J11 deferral comment in `internal/recurring/service.go` (mentions removed price-cap/plan-window checks).

- [ ] CLAUDE.md: slice list drops `customitem`/`pricelist`, adds `catalogue`; the "Price list" section becomes "Catalogue" describing the per-item copy-on-write model.
- [ ] data-model.md: ERD drop `custom_items`/`items`/`price_list_versions`, add `catalogue_items`, update line-item FKs to `catalogue_item_id`.
- [ ] Commit.

---

## Verification checklist (final)

- `CGO_ENABLED=0 go build ./cmd/tallyo` — cgo-free binary builds.
- `go test ./... -race` — green.
- `go vet ./...` ; `gofmt -l .` — clean.
- `cd web && npm run check` — 0 errors/0 warnings; `npm run build` — emits `web/build`.
- Manual: create a catalogue item → add it to an invoice line → edit the item's price → confirm the invoice keeps the old price (forked version) and the catalogue shows the new price.
