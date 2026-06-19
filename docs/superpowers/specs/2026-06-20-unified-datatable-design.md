# Unified DataTable + server-side query — design

**Date:** 2026-06-20
**Status:** Approved (design), pending implementation plan
**Topic:** Replace the ~8 hand-rolled list tables with one generic, Excel-like
DataTable component backed by server-side filter/sort/search/paginate.

## Problem

Every list page (`participants`, `invoices`, `estimates`, `custom-items`,
`tax-rates`, `plan-managers`, `recurring`, plus `ShiftTable`) hand-rolls its own
`<table>`: white card, gray-50 header, ad-hoc search box, inline-edit rows, and a
row of action buttons. There is no shared component, no column sort, no per-column
filter, and no row selection / bulk actions. All filtering that exists is
client-side over the full in-memory list (`createCollectionStore` loads every
row). This duplicates markup, drifts visually, and won't scale as row counts grow.

## Goals

- **One** table component used by every list page. A page declares its *columns*
  and *actions*; the component owns search, sort, filter, selection, pagination,
  and the edit drawer.
- **Excel-like**: per-column sort + filter + search, multi-row selection with
  bulk actions.
- **Server-driven**: search/filter/sort/paginate run in SQL; the browser holds
  one page at a time.
- **Notion-like open**: clicking a row opens a peek drawer for inline edit.
- Visual + interaction consistency across the whole app.

## Non-goals (YAGNI — add when asked)

- Column show/hide toggle.
- Saved views / saved filters.
- CSV export of the entire filtered set (export covers the current selection/page
  only for now).
- Drag-to-reorder columns, column resize, frozen columns.

## Frontend: `DataTable.svelte`

A single generic component (`web/src/lib/components/DataTable.svelte`). The page
supplies a typed config; the component renders everything else.

### Column + action config

```ts
type FilterType = 'text' | 'enum' | 'date' | 'number';

type Column<T> = {
  key: string;                 // the ONLY identifier the server accepts (allowlisted server-side)
  label: string;
  sortable?: boolean;
  filter?: FilterType;         // omit → no filter section in the menu
  values?: string[];          // enum options
  cell?: (row: T) => string;   // optional custom render (pills, formatted dates)
  icon?: Component;           // optional lucide icon in the header
};

type RowAction<T> = {
  label: string;
  icon?: Component;           // lucide
  run: (rows: T[]) => void | Promise<void>;
  danger?: boolean;
  bulk?: boolean;             // appears in the selection action set
};
```

### Layout

The table sits in a white rounded card. The card's top edge is a **control
strip**; the table body and a footer (row count + pager) follow.

**The strip has two orthogonal dimensions:**

- **Left = filters.** Always shows the table title. For each active filter, a
  chip (`Management: Plan-managed ✕`); when ≥1 filter is active, a `Clear filters`
  link. A column text-search counts as a filter and appears as a chip.
- **Right = selection.** Zero rows selected → the `New` button. ≥1 selected → the
  selection actions: `N selected`, the `bulk` row actions (each with its lucide
  icon; `danger` ones in red), and a `✕` to clear. Colors match the strip
  (dark text on the light strip — **not** a black block).

Both can be active at once (filtered *and* selected). Title always present.

### Column header menu (the funnel popover)

Clicking **anywhere on a column header** opens that column's menu. The header also
shows a small `▲`/`▼` indicator when it is the active sort, and a blue funnel icon
when a filter is active on it.

The menu is composed from the column config:

- If `sortable` → **Asc / Desc** as two side-by-side buttons (active one
  highlighted).
- Then the filter control, chosen by `filter` type:
  - `text` → a **Search** "contains…" input
  - `enum` → a **checkbox list** of `values`
  - `date` → **from / to** date inputs
  - `number` → **min / max** inputs
  - (none) → no filter section

All controls are **live**: changing any of them re-queries immediately. There is
**no Apply button**. There is **no Clear button in the menu** — clearing happens
via the strip chips / `Clear filters`.

### Selection

- Per-row checkbox + header select-all (select-all toggles the **current
  visible/queried page**).
- **Shift+click** a checkbox range-selects from the last toggled row.
- Selection survives paging within the same query? — No (keep simple): selection
  is page-scoped for v1; note as a known limitation.

### Peek drawer (open + edit)

- Clicking a row opens a right-side slide-in drawer (Notion-style) — **drawer
  everywhere**, even for resources that have a full detail page.
- Fields are generated from the columns by default; a page may pass a custom
  drawer slot for richer editing.
- Edits **debounce → save** and show a "✓ saved" affordance. Save reuses the
  existing `PUT /api/<resource>/:id` full-object update (no new PATCH endpoint
  needed); the component sends the full current record.
- Resources that have a detail route show an **"Open full page ↗"** link in the
  drawer header.

### Keyboard

- **Esc** — staged: closes the drawer first; else the open menu; else clears
  selection.
- **Shift+click** — range-select (above).
- **⌘/Ctrl+A** — select all visible rows (ignored while focus is in a filter
  input).
- Click outside the card (and not in the drawer) clears the selection.

### Icons

Use `@lucide/svelte` (already a dependency). No new icon dependency.

## Backend: server-side query

### API contract

Each list resource gains a queryable list endpoint (extend the existing
`GET /api/<resource>`):

```
GET /api/participants?sort=name&dir=asc&page=1&limit=50&f.name=lim&f.mgmt=plan,self&f.window.from=2025-06-01
→ { "rows": [ ...one page... ], "total": 124 }
```

Param encoding:
- `sort` = a column **key**; `dir` = `asc` | `desc`.
- `page` (1-based), `limit`.
- `f.<key>` = value. `text` → contains; `enum` → comma-separated set → `IN`.
- `f.<key>.from` / `f.<key>.to` for `date`/`number` ranges (`.min`/`.max` accepted
  as aliases).

There is no separate global `q` param — search is per-column (`f.<key>` text).

### Shared list helper — `internal/listquery`

A new platform package. Each slice's repository declares:

1. a **base SELECT** — a *constant* SQL string (the slice's existing enrichment
   query, with its joins), and
2. a **column spec map** — maps an API key → `{Col, Filter}`:

```go
var participantCols = listquery.Spec{
  "name":   {Col: "p.name",        Filter: listquery.Text},
  "ndis":   {Col: "p.ndis_number", Filter: listquery.Text},
  "mgmt":   {Col: "p.mgmt_type",   Filter: listquery.Enum},
  "window": {Col: "p.plan_start",  Filter: listquery.Date},
  "pm":     {Col: "pm.name",       Filter: listquery.Text},
}
```

The helper parses the request params against the spec, builds the WHERE / ORDER /
LIMIT *clauses*, appends them to the constant base SELECT, runs it, and runs a
`SELECT count(*) FROM (<base + where>)` for the total. Rows scan into the slice's
existing row struct.

### SQL-injection safeguards (the core requirement)

Dynamic SQL is only safe because identifiers never originate from the client:

1. **Identifiers come from the spec, never the client.** The client sends a
   *key* (`"mgmt"`); the helper looks it up and uses the **constant** `Col`
   string we authored. Unknown key → `400`, never interpolated. Same for `sort`.
2. **All values are bound `?` parameters** — `WHERE p.name LIKE ?` with `%term%`
   as an arg. Zero string concatenation of user values.
3. **Operators are fixed per filter type**, chosen by the helper, never by the
   client: `Text`→`LIKE`, `Enum`→`IN (?,?,…)`, `Date`/`Number`→`>= ? AND <= ?`.
4. **Sort direction** accepts only `asc`/`desc` → mapped to a constant; anything
   else falls back to `asc`.
5. **`limit`/`offset`** parsed as ints and clamped (`limit ≤ 200`, `offset ≥ 0`,
   `limit` default 50).
6. Only the WHERE/ORDER/LIMIT clauses are assembled from controlled fragments and
   appended to the constant base SELECT.

A unit test exercises the builder with hostile keys/values/dirs and asserts they
are rejected or bound, never interpolated.

### Why not sqlc here

sqlc generates static queries and cannot express dynamic WHERE/ORDER/LIMIT. The
base SELECT stays hand-written per slice (it already is, for enrichment joins);
`listquery` only appends safe, parameterized clauses. The rest of the codebase
keeps using sqlc as today.

## Store integration

`createCollectionStore` gains a `query(params) → {rows, total}` method that calls
the new endpoint; `crud` gains a matching `query`. The store holds the current
page + total + the live query params. SSE entity invalidation **re-runs the
current query** (instead of reloading the full list). The plain `list()` path
stays for any caller that still wants everything.

## Rollout

1. Build `internal/listquery` + its tests.
2. Build `DataTable.svelte` + the drawer + store `query()`.
3. Migrate **participants** first as the reference page (it has a detail route, an
   enum column, and inline edit — exercises every feature).
4. Migrate the rest: invoices, estimates, custom-items, tax-rates, plan-managers,
   recurring, then ShiftTable.
5. Delete the per-page hand-rolled tables and drop the
   `@careswitch/svelte-data-table` dependency once nothing imports it.

## Risks / open points

- **Date columns** rendered as a "window" (start–end) need a sensible filter
  target (filter on `plan_start`). Confirm per such column during migration.
- **Selection is page-scoped** in v1 (cleared on page change) — acceptable;
  revisit if bulk-across-pages is requested.
- **Drawer autosave via full PUT** re-sends the whole record; fine at these sizes.
  A real PATCH is a later optimization, not needed now.
