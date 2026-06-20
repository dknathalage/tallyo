# Route-based edit pages with debounced autosave

**Date:** 2026-06-20
**Status:** Approved (design)

## Problem

Today, clicking a row in the generic `DataTable` opens an in-place "peek drawer"
(a fixed sidebar) for editing. Editing context is ephemeral — it cannot be
navigated back to, deep-linked, or bookmarked, and the drawer can only edit flat
scalar fields. The bespoke Shifts table opens a modal instead, so click behaviour
is inconsistent across the app.

We want clicking a row (and the "+ New" button) to open a **dedicated edit/new
page** that is a real route — navigable with the browser back button,
deep-linkable, and refreshable. Edits **autosave** with a clear save status. This
becomes the default interaction for all tables.

## Constraints (context that shapes the design)

- **Static SPA.** The frontend uses `@sveltejs/adapter-static` (SPA, `200.html`
  fallback). There is **no SvelteKit server / form actions / progressive
  enhancement**. All data flows over a JSON REST API (`/api/...`) with
  Server-Sent Events for invalidation.
- **No form library.** Heavyweight SvelteKit form libraries (Superforms,
  Formsnap) are built around server form actions and are a misfit here. Decision:
  **no form library** — generate forms from the existing `Column[]` metadata that
  `DataTable` already defines, with small per-column validators. Zero new
  dependencies.
- **Existing shared plumbing to reuse:**
  - `createCollectionStore<T, TInput>(resource, entity)` → exposes `crud`,
    `items`, `rows`, `query`, `load`, `ensureSubscribed`
    (`web/src/lib/stores/collection.svelte.ts`).
  - `Crud<T, TInput>` already has `get(id)`, `create(input)`, `update(id, input)`,
    `remove(id)` (`web/src/lib/api/crud.ts`). No API additions required.
  - `DataTable.svelte` already owns the `Column` type (label, key, `filter`
    kind incl. `enum` + `values`), sort/filter/select/bulk-delete.

## Goals

1. Row click and "+ New" navigate to a dedicated route, back-navigable and
   deep-linkable.
2. One generic edit-page **base component** drives every table's edit/new page.
3. Edits autosave (debounced) with a clear per-page status line.
4. Consistent across all tables, including replacing the Shifts modal pathway
   over time (rich entities keep their extra functionality).

## Non-goals

- No global/offline save **queue panel**, no app-wide pending-saves widget.
- No blocking "unsaved changes" navigation guard.
- No optimistic cross-row caching beyond what the collection store already does.
- No new backend endpoints; no new runtime dependencies.

## Architecture

### `EntityEditor.svelte` (new, generic)

The single edit/new page body. Props:

| Prop | Type | Purpose |
|------|------|---------|
| `title` | `string` | Page heading + status context |
| `columns` | `Column[]` | Reused from `DataTable`; one input per column |
| `store` | collection store | Provides `crud` (`get`/`create`/`update`) |
| `id` | `number \| 'new'` | Edit an existing record or create a new one |
| `toInput` | `(row) => TInput` | Maps the editable row shape → API input |
| `backHref` | `string` | Where the ← link returns (the list route) |
| `validate?` | `(key, value) => string \| null` | Optional per-field validation |
| `extras?` | snippet | Entity-specific sections (line items, payments, …) |

Renders:
- a **back link** (`← {title list}`) — a plain `<a href={backHref}>`, so browser
  history/back works natively;
- one **field per column**: `text` default, `number` for numeric columns, `date`
  for date columns, `<select>` for `enum` columns (options from `col.values`);
- inline validation error under any invalid field;
- a **save-status line**: `saving… / ✓ saved / ⚠ error · retry`;
- the optional `extras` snippet below the fields.

### Routing convention

- `/{resource}/{id}` — edit an existing record.
- `/{resource}/new` — create a new record.

Real SvelteKit dynamic `[id]` routes (`+page.svelte` under
`src/routes/{resource}/[id]/`). These are SPA client routes; navigation uses
`goto`, which pushes browser history, so **back works with no extra state**.
Landing directly on an edit URL (deep link / refresh) fetches the record via
`store.crud.get(id)`; `new` starts from an empty draft.

`id === 'new'` is represented by the literal route segment `new`; the page
component detects it and passes `id: 'new'` to `EntityEditor`.

### `DataTable.svelte` change

Remove the peek drawer entirely: delete `drawerRow`, `draft`, `saveState`,
`saveTimer`, `openDrawer`, `closeDrawer`, `editField`, the `onRowSave` prop, and
the drawer markup block. Replace with navigation:

- new props `rowHref(row) => string` and `newHref: string`;
- row `onclick` → `goto(rowHref(row))`;
- `+ New` → `goto(newHref)` (keep the existing `onNew` escape hatch only if a
  caller still needs it; otherwise drop it).

Sort, per-column filters, selection, and bulk delete are unchanged. The
checkbox cell keeps `stopPropagation` so selecting a row does not navigate.

## Autosave (simple debounced)

Lives inside `EntityEditor`:

1. `draft` seeded from the loaded record (or `{}` for `new`).
2. Editing a field updates `draft[key]` and schedules a **400ms debounced**
   flush (a single shared timer; later edits reset it — edits coalesce).
3. **Flush:**
   - edit mode → `await store.crud.update(id, toInput(draft))`.
   - new mode → the first valid flush `await store.crud.create(toInput(draft))`,
     then `replaceState` the URL to `/{resource}/{newId}` and switch internal
     mode to edit, so subsequent flushes update rather than re-create.
   - **Single in-flight:** a `saving` flag guards re-entry; if a field changes
     while a save is in flight, mark `dirty` and re-flush once the save resolves.
4. **Status line** reflects flush state: `saving…`, then `✓ saved`, or
   `⚠ error · retry` (retry re-runs the last flush). No queue, no blocking guard.
5. **Best-effort on unmount:** if a debounce timer is pending when the component
   unmounts, fire the flush immediately (fire-and-forget) so a just-typed edit is
   not dropped. This is *not* a navigation guard — it never blocks leaving. The
   status line is gone by unmount, so a failure from this flush is **silently
   swallowed** (best-effort, accepted); the user's next visit shows the stored
   value. (`replaceState` for the create→update swap comes from `$app/navigation`,
   which currently only imports `goto` — add the import.)
6. **Validation:** before including a field in `toInput`, run `validate(key,
   value)`; an invalid field shows its error inline and is **withheld** from the
   payload. A required-but-missing field blocks `create` (status stays idle with
   the error shown) but never blocks navigation.

### SSE interaction

While the editor is open, an incoming SSE invalidation for the entity must not
clobber the in-progress `draft`. The editor holds its own `draft` state
independent of `store.items`; it only reads the store/`crud.get` on initial load.
Returning to the list re-runs the store's last query (existing behaviour), so the
edited row is fresh on return.

## Generic base, rich entities keep their guts

- **Simple CRUD** (`tax-rates`, `custom-items`, `plan-managers`, `recurring`):
  thin route files — `src/routes/{resource}/[id]/+page.svelte` renders
  `<EntityEditor columns store toInput backHref/>` with no `extras`. The list
  page passes `rowHref`/`newHref` to `DataTable`.
- **Rich** (`invoices`, `estimates`, `participants`): the same `EntityEditor`
  base drives the flat header fields + autosave; their rich sections (invoice/
  estimate line items, payments, PDF actions; a participant's shifts) are passed
  through the **`extras` snippet** so no functionality is lost. `invoices/[id]`
  and `participants/[id]` already exist and are refactored onto `EntityEditor`;
  `estimates` gains an `estimates/[id]` route.

### Sequencing (de-risks the rich-page migration)

1. Build `EntityEditor` + the autosave helper + tests.
2. Convert `DataTable` to navigation; wire the **4 simple CRUD** tables and their
   new `[id]` routes. Verify end-to-end.
3. Migrate rich entities **one at a time**, each verified: `invoices` →
   `estimates` → `participants`.
4. Shifts: the bespoke `ShiftTable` + `ShiftForm` modal is out of scope for the
   first pass (its record-flow / line-items / AI-divide needs exceed a flat
   editor). Tracked as follow-up; the modal stays until its rich sections are
   ported into an `extras` snippet.

## Data flow

```
List (DataTable)
   └─ row click → goto(/{resource}/{id})
        └─ EntityEditor: load (store.items hit or crud.get(id))
             └─ field edit → debounced crud.update → server → SSE
   ← browser back → list (store re-runs last query, row is fresh)
```

## Error handling

- **Save failure:** status → `⚠ error · retry`; `draft` retained; retry re-runs
  the flush.
- **Load failure** (bad/deleted id): show an error message + back link instead of
  the form.
- **Validation failure:** inline per-field error; invalid field withheld;
  `create` blocked until required fields valid.

## Testing

- **Vitest unit test** on the autosave helper (extracted as a small testable
  function/store, no DOM): debounce coalescing (rapid edits → one flush),
  create-then-switch-to-update transition, single-in-flight re-flush on
  mid-save edit, and per-column validator gating.
- `svelte-check` clean (0 errors / 0 warnings) per repo gate.
- Manual verification per migrated entity (create, edit, back-nav, deep-link,
  save-error retry).

## Open risks

- Rich-entity migration (`invoices`/`estimates`/`participants`) is the heavy,
  regression-prone part; staged last and verified individually.
- `replaceState` after first create must keep the in-flight `saving`/`dirty`
  state coherent so a fast typist's edits during the create→update swap are not
  lost (covered by the single-in-flight + re-flush logic and a unit test).
