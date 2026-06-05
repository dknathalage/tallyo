# Catalog Import Redesign — Inline Auto-Mapping

**Date:** 2026-06-05
**Status:** Approved (design)

## Problem

Catalog import today is a two-page, manual affair:

- `/column-mappings` — a persisted, separately-managed entity (`column_mappings`
  table + repo + service + handler + Svelte store + route). You must **create
  and save a named mapping first**.
- `/import` — an upload wizard that references a saved mapping **by id**
  (`mappingId` form field). The backend (`internal/http/import.go`) resolves the
  id, then parses → maps → diffs → commits.

This forces the user to hand-build and persist a column mapping before they can
import anything. The goal is the opposite: **upload a file and have the columns
mapped automatically at import time**, with the user only confirming/adjusting —
no saved-mapping management at all. It must work for **any** catalog CSV/XLSX
(NDIS price catalogues are one example, not a special case), including per-tier
prices.

## Goals

- Delete the persisted column-mapping feature entirely.
- Fold import into the **Catalog** section of the app.
- **Auto-detect** the column mapping from the uploaded file's headers + sampled
  values, pre-filling an inline review step.
- Handle **multiple price columns**: one base rate + the rest as rate tiers,
  where the user **selects** the target tier per column (existing tier, create a
  new tier with an editable name, or ignore).
- Keep the existing parse → map → diff → commit pipeline and its safety (preview
  diff before commit).

## Non-Goals

- No saved/reusable mappings, no "remember last mapping" — mapping is ephemeral
  per import.
- No NDIS-specific preset or hardcoded NDIS column list. Detection is generic.
- No new file formats (CSV + XLSX only, as today).
- No change to catalog/tier/rate **schema** beyond dropping `column_mappings`.

## Design

### Data model

- **Drop** the `column_mappings` table via a new goose migration
  (`internal/db/migrations/00010_drop_column_mappings.sql`). Down re-creates it
  (copy the `CREATE TABLE` from `00009`).
- `rate_tiers`, `catalog_items`, `catalog_item_rates` are unchanged. Auto-created
  tiers are ordinary `rate_tiers` rows created through the audited path.

### Detection (`internal/importer`)

New `DetectMapping(headers []string, sample []map[string]string) Suggestion`.

```go
type Suggestion struct {
    Fields     map[string]string   // header -> one of name|sku|unit|category|rate
    PriceCols  []PriceColumn       // every detected price column except the base
    BaseHeader string              // header chosen as the base catalog rate
}

type PriceColumn struct {
    Header      string // source column header
    SuggestName string // proposed tier name (default: the header), editable
}
```

Algorithm (deterministic, pure-Go):

1. **Normalize** each header: lower-case, trim, collapse non-alphanumerics.
2. **Field match** against a synonym table, first match wins, each field claimed
   at most once:
   - `name` ← name, item, item name, support item name, description, product, service
   - `sku` ← sku, code, item number, support item number, item code, id, ref
   - `unit` ← unit, uom, unit of measure
   - `category` ← category, group, support category, type, class
   - base `rate` ← price, rate, cost, amount, unit price, base price, price limit
3. **Price-column detection.** A column is "price-like" if its header matches a
   price synonym **OR** a majority of its sampled non-empty cells parse as
   currency (a `$` sign, or a decimal with ≤2 fraction digits). Integer-only code
   columns (e.g. a numeric category number) are **not** price-like, so they never
   become junk tiers.
4. **Base vs tiers.** Among price-like columns, the base rate is the one matched
   to `rate` in step 2; if none matched, the **left-most** price-like column.
   Every other price-like column becomes a `PriceColumn` with `SuggestName =
   header`.
5. Required field is **`name`**. If detection cannot find a `name` column, the
   suggestion still returns (with `name` unmapped) and the UI flags it; the user
   maps it manually. `sku` is optional but, when absent, the diff keys nothing
   and all rows are treated as new (existing behaviour).

### Mapping is now transient

`ApplyMapping` currently takes `*repository.ColumnMapping` (a DB row). Replace
with a transient value built from the request:

```go
type Mapping struct {
    Fields    map[string]string  // header -> field
    TierCols  map[string]string  // header -> tier name (only kept columns)
    FileType  string
    SheetName string
    HeaderRow int
}
```

- `ApplyMapping(rows, Mapping)` produces `[]MappedRow` + `[]RowError`.
- `MappedRow.TierRates` changes from `map[int64]float64` (tier **id**) to
  `map[string]float64` (tier **name**) — ids do not exist at map time because a
  tier may still need creating.

### Commit resolves tier names → ids (create-if-missing)

`Commit` (and its `applyTierRates` helper) resolve each tier **name** to an
existing `rate_tier` (case-insensitive, unique name) or **create** it through the
audited repository path, then write `catalog_item_rates`. Only columns the user
kept (present in `Mapping.TierCols`) are processed; "ignore" columns are dropped
client-side and never reach the backend.

### HTTP (`internal/http`)

Move import under the catalog route group and pass the mapping **inline** (JSON),
not by id. `ImportHandler` drops its `*ColumnMappingsRepo` dependency.

- `POST /api/catalog/import/parse` — multipart `file` (+ optional `fileType`,
  `sheetName`, `headerRow`). Parses, samples up to N rows, runs `DetectMapping`,
  returns `{ headers, sample, suggestion }`. Writes nothing.
- `POST /api/catalog/import/preview` — multipart `file` + `mapping` (JSON string)
  → `DiffResult`. Writes nothing.
- `POST /api/catalog/import/commit` — multipart `file` + `mapping` (JSON) +
  `updateExisting` → `CommitResult`. The only mutating route; creates tiers +
  catalog items through the audited path and broadcasts SSE as today.

Both preview and commit re-parse the uploaded file (stateless), matching the
current design.

### Removals

- Backend: `internal/http/column_mappings.go`,
  `internal/service/column_mapping.go`, `internal/repository/column_mapping.go`,
  `internal/db/queries/column_mappings.sql`; regenerate sqlc
  (`internal/db/gen` loses the column-mapping methods). Unwire `ColumnMappings`
  from `internal/http/server.go` `Deps` + routes and from `main.go`
  construction. `NewImportHandler` loses its `mappings` parameter.
- Frontend: delete `web/src/routes/column-mappings/`,
  `web/src/lib/stores/columnMappings.svelte.ts`, the nav link, and any
  column-mapping types in `web/src/lib/api/types.ts`.

### Web UI

- `web/src/routes/catalog/import/` — a wizard with three steps:
  1. **Upload** — pick file (+ advanced: sheet, header row). Calls `/parse`.
  2. **Review** — a table of detected columns. Field selects (name/sku/unit/
     category/rate/ignore) pre-filled from `suggestion.Fields`. For each
     `PriceColumn`, a **tier select**: options = existing `rate_tiers` (from the
     existing `/api/rate-tiers` list) + **"Create new tier…"** (reveals an
     editable name input, default = header) + **"Ignore"**. `name` flagged if
     unmapped — cannot proceed until mapped. Calls `/preview`.
  3. **Diff & commit** — show `Summary` (new/updated/unchanged/errors), an
     `updateExisting` toggle, Commit button → `/commit`.
- An **Import** button on `/catalog` links to the wizard. After a successful
  commit, return to `/catalog`; SSE refreshes the list.

## Testing

- `DetectMapping` table tests:
  - NDIS-style headers (Support Item Number/Name, Support Category, Unit, +
    geographic price columns) → correct fields, base rate, geographic columns as
    `PriceColumn`s.
  - Generic `name,sku,unit,price,premium` → `premium` is a tier column.
  - A numeric **code** column (integers, no `$`/decimals) is **not** detected as
    a price/tier column.
  - Missing `name` column → suggestion returns with `name` unmapped (no panic).
- `ApplyMapping` with a transient `Mapping` (tier names) → `MappedRow.TierRates`
  keyed by name.
- `Commit` test: a tier name with no existing row is **created** once and the
  `catalog_item_rates` row is written; an existing tier name reuses its id; tiers
  are created through the audited path.
- HTTP tests for `/parse`, `/preview`, `/commit` (happy path + missing file +
  unmapped `name`).
- Migration round-trip: up drops the table, down restores it.

## Risks / Notes

- **Currency sniffing** is locale-light (handles `$`, `,` grouping, `.`
  decimals — matching the existing lenient `parseFloat`). Non-`$` currencies or
  comma-decimals may mis-sniff; the inline review (toggle a column to "ignore" or
  off-tier) is the safety net.
- Tier names are unique in `rate_tiers`; resolving by name and creating-if-missing
  is the single source of truth, so existing-vs-new collapses to one code path.
- Clean-break migration policy is respected: a forward `00010` migration, no edit
  of `00009`.
