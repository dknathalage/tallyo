# Route-based edit pages with debounced autosave — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Clicking a table row (or "+ New") opens a dedicated, back-navigable, deep-linkable edit/new **page** whose fields autosave with a clear status line — replacing the in-place peek drawer across all tables.

**Architecture:** One generic `EntityEditor.svelte` renders fields from the existing `Column[]` metadata and drives a framework-agnostic `createAutosave` helper (debounce + single-in-flight + create→update swap). `DataTable` stops opening a drawer and instead navigates (`goto`) to `/{resource}/{id}` or `/{resource}/new`. Simple CRUD entities get thin route files; rich entities (invoices/estimates/participants) compose `EntityEditor` with their existing sections passed through an `extras` snippet. Build order de-risks the rich migrations by doing them last, one at a time.

**Tech Stack:** Svelte 5 runes, SvelteKit (`adapter-static` SPA), TypeScript, Tailwind 4, Vitest. JSON REST API via `crud` + SSE. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-06-20-route-based-edit-pages-autosave-design.md`

---

## File structure

| File | Responsibility |
|------|----------------|
| `web/src/lib/components/datatable.ts` (modify) | Add optional `input` field to `Column<T>` so the editor knows how to render each field. |
| `web/src/lib/components/autosave.ts` (create) | Framework-agnostic debounced-save state machine. Pure TS, unit-tested. |
| `web/src/lib/components/autosave.test.ts` (create) | Vitest unit tests for the state machine. |
| `web/src/lib/components/EntityEditor.svelte` (create) | Generic edit/new page body: load record, render fields, wire autosave + status line, `extras` slot. |
| `web/src/lib/components/DataTable.svelte` (modify) | Remove peek drawer; row-click + "+ New" navigate via `rowHref`/`newHref`. |
| `web/src/routes/{tax-rates,custom-items,plan-managers,recurring}/[id]/+page.svelte` (create) | Thin route files rendering `EntityEditor` for each simple CRUD entity. |
| `web/src/routes/{tax-rates,custom-items,plan-managers,recurring}/+page.svelte` (modify) | Drop in-page create Modal; pass `rowHref`/`newHref` to `DataTable`. |
| `web/src/routes/invoices/[id]/+page.svelte`, `estimates/[id]/+page.svelte` (create/modify), `participants/[id]/+page.svelte` (modify) | Rich entities composed on `EntityEditor` via `extras`; their list pages switch to navigation. |

**Out of scope (follow-up):** the bespoke `ShiftTable` + `ShiftForm` modal. The Shifts page keeps its modal; not migrated in this plan.

---

## Task 1: Extend the `Column` type with an edit-input hint

**Files:**
- Modify: `web/src/lib/components/datatable.ts`

- [ ] **Step 1: Add the `input` field to `Column<T>`**

In `datatable.ts`, add an `EditInput` type and an optional `input` property. A column with no `input` is inferred from `filter` (`number`→number, `date`→date, `enum`→select, else text). `'readonly'` renders the value but no input. Booleans/long-text set it explicitly.

```ts
/** How a column renders in the EntityEditor form. Inferred from `filter` when omitted. */
export type EditInput = 'text' | 'textarea' | 'number' | 'date' | 'select' | 'checkbox' | 'readonly';

export interface Column<T> {
	key: string;
	label: string;
	sortable?: boolean;
	filter?: FilterType;
	values?: string[]; // enum options (also used as <select> options when input==='select')
	cell?: (row: T) => string;
	/** Edit-page input kind. Omit to infer from `filter`. Use 'readonly' for derived/non-editable columns. */
	input?: EditInput;
}
```

- [ ] **Step 2: Verify type-checks**

Run: `cd web && npm run check`
Expected: 0 errors / 0 warnings (purely additive optional field).

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/datatable.ts
git commit -m "feat(web): add optional edit-input hint to DataTable Column"
```

---

## Task 2: `createAutosave` state machine (TDD)

The trickiest logic lives here, isolated and DOM-free so it is unit-testable: debounce coalescing, single in-flight save, create-once-then-update, and manual retry.

**Files:**
- Create: `web/src/lib/components/autosave.ts`
- Test: `web/src/lib/components/autosave.test.ts`

- [ ] **Step 1: Write the failing tests**

```ts
// web/src/lib/components/autosave.test.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createAutosave, type SaveState } from './autosave';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

type Payload = { name: string };
type Row = { id: number; name: string };

function harness(overrides: Partial<{ createImpl: (p: Payload) => Promise<Row> }> = {}) {
	const states: SaveState[] = [];
	const created: number[] = [];
	const create = vi.fn(overrides.createImpl ?? (async (p: Payload) => ({ id: 1, ...p })));
	const update = vi.fn(async (_id: number, p: Payload) => ({ id: 1, ...p }));
	const a = createAutosave<Payload, Row>({
		create,
		update,
		delay: 400,
		onState: (s) => states.push(s),
		onCreated: (id) => created.push(id)
	});
	return { a, create, update, states, created };
}

describe('createAutosave', () => {
	it('coalesces rapid edits into a single save', async () => {
		const { a, create } = harness();
		a.schedule({ name: 'a' });
		a.schedule({ name: 'b' });
		vi.advanceTimersByTime(400);
		await vi.runAllTimersAsync();
		expect(create).toHaveBeenCalledTimes(1);
		expect(create).toHaveBeenCalledWith({ name: 'b' });
	});

	it('creates once, then updates with the captured id', async () => {
		const { a, create, update, created } = harness();
		a.schedule({ name: 'a' });
		await vi.runAllTimersAsync();
		expect(create).toHaveBeenCalledTimes(1);
		expect(created).toEqual([1]);
		a.schedule({ name: 'b' });
		await vi.runAllTimersAsync();
		expect(update).toHaveBeenCalledWith(1, { name: 'b' });
		expect(create).toHaveBeenCalledTimes(1);
	});

	it('serializes a mid-flight edit into one follow-up save', async () => {
		let resolveCreate!: (r: Row) => void;
		const createImpl = (_p: Payload) => new Promise<Row>((res) => (resolveCreate = res));
		const { a, update } = harness({ createImpl });
		a.schedule({ name: 'a' });
		await vi.advanceTimersByTimeAsync(400); // flush → create in flight
		a.schedule({ name: 'b' }); // arrives mid-flight
		await vi.advanceTimersByTimeAsync(400);
		resolveCreate({ id: 1, name: 'a' });
		await vi.runAllTimersAsync();
		expect(update).toHaveBeenCalledTimes(1);
		expect(update).toHaveBeenCalledWith(1, { name: 'b' });
	});

	it('reports error then retries the failed payload', async () => {
		const create = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({ id: 1, name: 'a' });
		const states: SaveState[] = [];
		const a = createAutosave<Payload, Row>({
			create,
			update: vi.fn(async (id, p) => ({ id, ...p })),
			delay: 400,
			onState: (s) => states.push(s)
		});
		a.schedule({ name: 'a' });
		await vi.runAllTimersAsync();
		expect(states).toContain('error');
		a.retry();
		await vi.runAllTimersAsync();
		expect(create).toHaveBeenCalledTimes(2);
		expect(states).toContain('saved');
	});
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/lib/components/autosave.test.ts`
Expected: FAIL — `Cannot find module './autosave'`.

- [ ] **Step 3: Implement `createAutosave`**

```ts
// web/src/lib/components/autosave.ts

export type SaveState = 'idle' | 'saving' | 'saved' | 'error';

export interface AutosaveOptions<T, R extends { id: number }> {
	/** Persist a brand-new record; resolves to the created entity (carrying its id). */
	create: (payload: T) => Promise<R>;
	/** Persist an update to an already-created record. */
	update: (id: number, payload: T) => Promise<R>;
	/** Debounce window before a scheduled flush fires (ms). */
	delay?: number;
	/** Called on every state transition — drive the status line from this. */
	onState?: (state: SaveState) => void;
	/** Called once with the new id the first time a `new` record is created. */
	onCreated?: (id: number) => void;
}

export interface Autosave<T> {
	/** Debounced save of the latest payload (newer payloads supersede older). */
	schedule: (payload: T) => void;
	/** Flush a pending payload immediately. Safe to call fire-and-forget on teardown. */
	flush: () => void;
	/** Re-run the last failed payload (status-line retry button). */
	retry: () => void;
	/** Cancel the debounce timer, flushing any pending edit best-effort. */
	dispose: () => void;
}

/**
 * Debounced single-in-flight save machine. The FIRST successful save of a brand
 * new record calls `create` and captures the returned id; every later save (and
 * any edit that arrives while a save is in flight) calls `update` with that id.
 * Errors are surfaced via `onState('error')` and the failed payload is held for
 * `retry()` — never auto-retried (so a persistent failure can't spin).
 */
export function createAutosave<T, R extends { id: number }>(
	opts: AutosaveOptions<T, R>
): Autosave<T> {
	const delay = opts.delay ?? 400;
	let id: number | null = null; // null until the first create resolves
	let timer: ReturnType<typeof setTimeout> | null = null;
	let saving = false;
	let pending: T | null = null; // latest payload awaiting a flush
	let dirtyDuringSave = false; // an edit arrived while a save was in flight
	let lastFailed: T | null = null;

	function schedule(payload: T): void {
		pending = payload;
		if (timer) clearTimeout(timer);
		timer = setTimeout(flush, delay);
	}

	function flush(): void {
		if (timer) {
			clearTimeout(timer);
			timer = null;
		}
		if (pending === null) return;
		if (saving) {
			dirtyDuringSave = true;
			return;
		}
		const payload = pending;
		pending = null;
		saving = true;
		opts.onState?.('saving');
		const op = id === null ? opts.create(payload) : opts.update(id, payload);
		void op
			.then((row) => {
				if (id === null) {
					id = row.id;
					opts.onCreated?.(row.id);
				}
				opts.onState?.('saved');
			})
			.catch(() => {
				lastFailed = payload;
				opts.onState?.('error');
			})
			.finally(() => {
				saving = false;
				if (dirtyDuringSave) {
					dirtyDuringSave = false;
					flush();
				}
			});
	}

	function retry(): void {
		if (lastFailed === null) return;
		pending = lastFailed;
		lastFailed = null;
		flush();
	}

	function dispose(): void {
		if (timer) {
			clearTimeout(timer);
			timer = null;
			flush(); // best-effort; never blocks
		}
	}

	return { schedule, flush, retry, dispose };
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/lib/components/autosave.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/components/autosave.ts web/src/lib/components/autosave.test.ts
git commit -m "feat(web): debounced single-in-flight autosave state machine"
```

---

## Task 3: `EntityEditor.svelte` generic edit/new page

**Files:**
- Create: `web/src/lib/components/EntityEditor.svelte`

- [ ] **Step 1: Write the component**

Props are generic over the row type `T` and its input type `TInput`. It loads the record (or starts blank for `new`), renders one control per column, runs optional validation, and feeds valid payloads into `createAutosave`. `extras` renders entity-specific sections below the fields.

```svelte
<!-- web/src/lib/components/EntityEditor.svelte -->
<script lang="ts" generics="T extends { id: number }, TInput">
	import type { Snippet } from 'svelte';
	import { onDestroy } from 'svelte';
	import { goto, replaceState } from '$app/navigation';
	import type { Column, EditInput } from './datatable';
	import { createAutosave, type SaveState } from './autosave';

	type Crud = {
		get: (id: number) => Promise<T>;
		create: (input: TInput) => Promise<T>;
		update: (id: number, input: TInput) => Promise<T>;
	};

	type Props = {
		title: string;
		columns: Column<T>[];
		crud: Crud;
		/** Existing record id, or 'new' to create. */
		id: number | 'new';
		/** Map the editable draft back to the API input shape. */
		toInput: (draft: T) => TInput;
		/** List route the back-link returns to (e.g. '/tax-rates'). */
		backHref: string;
		/** Starting values for a 'new' record; merged over inferred defaults. */
		blank?: Partial<T>;
		/** Optional per-field validation. Return an error string, or null when valid. */
		validate?: (key: string, value: unknown, draft: T) => string | null;
		/** Entity-specific sections rendered below the fields (rich entities). */
		extras?: Snippet<[T]>;
	};

	let { title, columns, crud, id, toInput, backHref, blank, validate, extras }: Props = $props();

	function inputKind(col: Column<T>): EditInput {
		if (col.input) return col.input;
		if (col.filter === 'number') return 'number';
		if (col.filter === 'date') return 'date';
		if (col.filter === 'enum') return 'select';
		return 'text';
	}

	function defaultFor(col: Column<T>): unknown {
		switch (inputKind(col)) {
			case 'number':
				return 0;
			case 'checkbox':
				return false;
			case 'select':
				return col.values?.[0] ?? '';
			default:
				return '';
		}
	}

	// ── Load / seed draft ──────────────────────────────────────────────────────
	let draft = $state<T | null>(null);
	let loadError = $state<string | null>(null);
	let recordId = $state<number | 'new'>(id);

	async function init(): Promise<void> {
		if (id === 'new') {
			const seed: Record<string, unknown> = { id: 0 };
			for (const col of columns) seed[col.key] = defaultFor(col);
			draft = { ...(seed as T), ...(blank ?? {}) };
			return;
		}
		try {
			draft = await crud.get(id);
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to load record.';
		}
	}
	void init();

	// ── Autosave wiring ─────────────────────────────────────────────────────────
	let saveState = $state<SaveState>('idle');
	const autosave = createAutosave<TInput, T>({
		create: (input) => crud.create(input),
		update: (existingId, input) => crud.update(existingId, input),
		onState: (s) => (saveState = s),
		onCreated: (newId) => {
			recordId = newId;
			replaceState(`${backHref}/${newId}`, {});
		}
	});
	onDestroy(() => autosave.dispose());

	let errors = $state<Record<string, string | null>>({});

	function edit(col: Column<T>, value: unknown): void {
		if (!draft) return;
		draft = { ...draft, [col.key]: value };
		const msg = validate ? validate(col.key, value, draft) : null;
		errors = { ...errors, [col.key]: msg };
		// Withhold the whole save while any field is invalid.
		if (Object.values(errors).some((e) => e)) return;
		autosave.schedule(toInput(draft));
	}

	function num(v: string): number {
		const n = Number(v);
		return Number.isFinite(n) ? n : 0;
	}
</script>

<div class="space-y-5">
	<div class="flex items-center justify-between">
		<a href={backHref} class="text-sm text-gray-500 hover:text-gray-900">← Back</a>
		<span class="h-4 text-xs">
			{#if saveState === 'saving'}<span class="text-gray-400">saving…</span>
			{:else if saveState === 'saved'}<span class="text-green-600">✓ saved</span>
			{:else if saveState === 'error'}
				<span class="text-red-600"
					>⚠ error ·
					<button type="button" class="underline" onclick={() => autosave.retry()}>retry</button>
				</span>
			{/if}
		</span>
	</div>

	<h1 class="text-xl font-semibold">{recordId === 'new' ? `New ${title}` : title}</h1>

	{#if loadError}
		<p class="text-sm text-red-600">{loadError}</p>
	{:else if !draft}
		<p class="text-sm text-gray-500">Loading…</p>
	{:else}
		<div class="max-w-xl space-y-4">
			{#each columns as col (col.key)}
				{@const kind = inputKind(col)}
				<label class="block">
					<span class="mb-1 block text-sm font-medium">{col.label}</span>
					{#if kind === 'readonly'}
						<p class="text-sm text-gray-600">{col.cell ? col.cell(draft) : String((draft as Record<string, unknown>)[col.key] ?? '')}</p>
					{:else if kind === 'checkbox'}
						<input
							type="checkbox"
							checked={Boolean((draft as Record<string, unknown>)[col.key])}
							onchange={(e) => edit(col, e.currentTarget.checked)}
							class="h-4 w-4"
						/>
					{:else if kind === 'select'}
						<select
							value={String((draft as Record<string, unknown>)[col.key] ?? '')}
							onchange={(e) => edit(col, e.currentTarget.value)}
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						>
							{#each col.values ?? [] as opt (opt)}<option value={opt}>{opt}</option>{/each}
						</select>
					{:else if kind === 'textarea'}
						<textarea
							value={String((draft as Record<string, unknown>)[col.key] ?? '')}
							oninput={(e) => edit(col, e.currentTarget.value)}
							rows="3"
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						></textarea>
					{:else}
						<input
							type={kind === 'number' ? 'number' : kind === 'date' ? 'date' : 'text'}
							value={String((draft as Record<string, unknown>)[col.key] ?? '')}
							oninput={(e) =>
								edit(col, kind === 'number' ? num(e.currentTarget.value) : e.currentTarget.value)}
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						/>
					{/if}
					{#if errors[col.key]}<span class="mt-1 block text-xs text-red-600">{errors[col.key]}</span>{/if}
				</label>
			{/each}
		</div>

		{#if extras}{@render extras(draft)}{/if}
	{/if}
</div>
```

- [ ] **Step 2: Type-check**

Run: `cd web && npm run check`
Expected: 0 errors / 0 warnings.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/EntityEditor.svelte
git commit -m "feat(web): generic EntityEditor edit/new page with autosave"
```

---

## Task 4: Convert `DataTable` from drawer to navigation

**Files:**
- Modify: `web/src/lib/components/DataTable.svelte`

- [ ] **Step 1: Replace drawer props with nav props**

In the `Props` type, remove `onRowSave` and `detailHref`; add `rowHref` and `newHref`. Keep `onNew` removed (the "+ New" button now navigates).

```ts
import { goto } from '$app/navigation';
// …
type Props = {
	title: string;
	columns: Column<T>[];
	store: Store;
	rowActions?: RowAction<T>[];
	/** Row click target, e.g. (r) => `/tax-rates/${r.id}`. */
	rowHref: (row: T) => string;
	/** "+ New" target, e.g. '/tax-rates/new'. */
	newHref: string;
	pageSize?: number;
};

let { title, columns, store, rowActions = [], rowHref, newHref, pageSize = 50 }: Props = $props();
```

- [ ] **Step 2: Delete the drawer state + functions**

Remove `drawerRow`, `draft`, `saveState`, `saveTimer`, `openDrawer`, `closeDrawer`, `editField`, and the `ExternalLink` import (no longer used). Keep the `Plus` import (still used by "+ New"). **Also** check the window click/key handlers (`onWindowClick`/`onWindowKey`, ~lines 248, 255–256) — they reference `drawerRow`/`closeDrawer`; remove those branches (they exist only to close the drawer on outside-click/Escape). After removal, grep the file for `drawerRow`/`closeDrawer`/`onRowSave` to confirm zero references, else `npm run check` will stay red.

- [ ] **Step 3: Make the "+ New" button navigate**

The "+ New" button currently renders only inside `{:else if onNew}`. Since `onNew` is gone and `newHref` is required, **flip that guard to `{:else}`** (or render the button unconditionally) and change its `onclick`:

```svelte
<button type="button" onclick={() => goto(newHref)} class="…unchanged classes…">
	<Plus class="size-4" /> New
</button>
```

- [ ] **Step 4: Make row click navigate**

Change the row open handler:

```svelte
<tr
	class="cursor-pointer border-t border-gray-100 {selected.has(row.id) ? 'bg-blue-50 hover:bg-blue-100' : 'hover:bg-gray-50'}"
	onclick={() => goto(rowHref(row))}
>
```

Leave the checkbox cell's `onclick={(e) => { e.stopPropagation(); … }}` intact so selecting never navigates.

- [ ] **Step 5: Delete the drawer markup block**

Remove the entire `<!-- peek drawer -->` `{#if drawerRow}…{/if}` block at the end of the markup.

- [ ] **Step 6: Type-check (expect call-site errors next task)**

Run: `cd web && npm run check`
Expected: errors ONLY at the 7 list-page call sites (they still pass `onRowSave`/`detailHref`/`onNew`). `DataTable.svelte` itself is clean. These call sites are fixed in Tasks 5–8.

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/components/DataTable.svelte
git commit -m "feat(web): DataTable navigates to edit page instead of opening drawer"
```

---

## Task 5: Wire `tax-rates` (first vertical slice — prove the pattern)

**Files:**
- Create: `web/src/routes/tax-rates/[id]/+page.svelte`
- Modify: `web/src/routes/tax-rates/+page.svelte`

- [ ] **Step 1: Mark the derived column editable/readonly**

In `tax-rates/+page.svelte`, update the columns so the editor renders correct inputs. `rate` is numeric, `isDefault` is a checkbox:

```ts
const columns: Column<TaxRate>[] = [
	{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
	{ key: 'rate', label: 'Rate', sortable: true, filter: 'number', cell: (t) => String(t.rate) },
	{ key: 'isDefault', label: 'Default', sortable: true, input: 'checkbox', cell: (t) => (t.isDefault ? 'Default' : '') }
];
```

- [ ] **Step 2: Create the edit/new route**

```svelte
<!-- web/src/routes/tax-rates/[id]/+page.svelte -->
<script lang="ts">
	import { page } from '$app/state';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import type { TaxRate, TaxRateInput } from '$lib/api/types';

	const columns: Column<TaxRate>[] = [
		{ key: 'name', label: 'Name', filter: 'text' },
		{ key: 'rate', label: 'Rate', input: 'number' },
		{ key: 'isDefault', label: 'Default', input: 'checkbox' }
	];

	function toInput(t: TaxRate): TaxRateInput {
		return { name: t.name, rate: t.rate, isDefault: t.isDefault };
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	const idParam = $derived(page.params.id === 'new' ? 'new' : Number(page.params.id));
</script>

<EntityEditor
	title="Tax rate"
	{columns}
	crud={taxRates.crud}
	id={idParam}
	{toInput}
	{validate}
	backHref="/tax-rates"
/>
```

- [ ] **Step 3: Strip the in-page create Modal; switch DataTable to navigation**

In `tax-rates/+page.svelte`: delete the `Modal` import + `new*`/`creating`/`showForm`/`formError` state, `openCreate`/`cancelCreate`/`resetNew`/`createTaxRate`, and the `<Modal>…</Modal>` block. Update the `<DataTable>` call:

```svelte
<DataTable
	title="Tax rates"
	{columns}
	store={taxRates}
	{rowActions}
	rowHref={(r) => `/tax-rates/${r.id}`}
	newHref="/tax-rates/new"
/>
```

- [ ] **Step 4: Type-check**

Run: `cd web && npm run check`
Expected: 0 errors / 0 warnings for `tax-rates/**` and `DataTable.svelte` (other unmigrated pages may still error — fixed in later tasks).

- [ ] **Step 5: Manual verification**

Run the app (`cd web && npm run build && cd .. && go run ./cmd/tallyo --port 8080`, or `cd web && npm run dev`). Confirm:
- Tax-rates list: clicking a row navigates to `/tax-rates/{id}`; "+ New" → `/tax-rates/new`.
- Editing a field shows `saving… → ✓ saved`; reload shows the persisted value.
- "+ New": first field edit creates the record and the URL swaps to `/tax-rates/{id}`; further edits update.
- Browser back returns to the list with the row updated.

- [ ] **Step 6: Commit**

```bash
git add web/src/routes/tax-rates
git commit -m "feat(web): tax-rates row click opens autosaving edit page"
```

---

## Task 6: Wire `custom-items`, `plan-managers`, `recurring` (repeat the pattern)

Repeat Task 5 for each. Read each list page first to copy its exact columns, `toInput`, store, and required-field rules. For each entity, do these steps and commit per entity.

**Files (per entity `<e>` ∈ {custom-items, plan-managers, recurring}):**
- Create: `web/src/routes/<e>/[id]/+page.svelte`
- Modify: `web/src/routes/<e>/+page.svelte`

- [ ] **Step 1 (custom-items):** Read `web/src/routes/custom-items/+page.svelte`. Create `custom-items/[id]/+page.svelte` mirroring Task 5 (columns with correct `input` kinds — currency/number fields → `'number'`, booleans → `'checkbox'`, long descriptions → `'textarea'`). Strip its create Modal; set `rowHref`/`newHref`. Run `npm run check`; manually verify create/edit/back. Commit `feat(web): custom-items autosaving edit page`.

- [ ] **Step 2 (plan-managers):** Same for `web/src/routes/plan-managers/+page.svelte`. Commit `feat(web): plan-managers autosaving edit page`.

- [ ] **Step 3 (recurring):** Same for `web/src/routes/recurring/+page.svelte`. Note: recurring currently edits via a form modal with a `rowActions` "Edit" button — remove that Edit action (row click replaces it); keep any non-edit row actions. Commit `feat(web): recurring autosaving edit page`.

- [ ] **Step 4: Full type-check + tests**

Run: `cd web && npm run check && npm test`
Expected: 0 errors / 0 warnings; autosave tests pass. All four simple CRUD entities now navigate to autosaving edit pages.

---

## Task 7: Migrate rich entities (invoices → estimates → participants)

Rich entities have sections a flat form can't hold (line items, payments, PDF actions; a participant's shifts). They compose `EntityEditor` for the flat header fields and pass everything else through the `extras` snippet. **Do one entity at a time, verify, then commit before starting the next.**

**Process per rich entity:**

- [ ] **Step 1 (invoices):**
  - Read `web/src/routes/invoices/+page.svelte` and `web/src/routes/invoices/[id]/+page.svelte`.
  - In `invoices/[id]/+page.svelte`, wrap the existing rich sections in an `{#snippet extras(row)} … {/snippet}` and render `<EntityEditor … {extras}>` for the editable header fields (status, dates, references). Keep line items / payments / PDF exactly as they are inside the snippet. Preserve any deep behaviour (don't autosave fields the API treats as derived — mark those columns `input: 'readonly'`).
  - In `invoices/+page.svelte`, replace `onRowSave`/`detailHref`/`onNew` with `rowHref={(r) => `/invoices/${r.id}`}` and `newHref="/invoices/new"` (or keep a custom new flow if invoices aren't created from a blank form — if so, point `newHref` at the existing creation entry point and note it).
  - `npm run check`; manually verify edit + back + each rich section still works.
  - Commit `feat(web): invoices edit page on EntityEditor with extras`.

- [ ] **Step 2 (estimates):**
  - Read `web/src/routes/estimates/+page.svelte` (it has no `[id]` route today).
  - Create `web/src/routes/estimates/[id]/+page.svelte` composing `EntityEditor` + an `extras` snippet for estimate line items (model it on the invoices `[id]` page).
  - Switch `estimates/+page.svelte` to `rowHref`/`newHref`.
  - `npm run check`; manual verify. Commit `feat(web): estimates autosaving edit page`.

- [ ] **Step 3 (participants):**
  - Read `web/src/routes/participants/+page.svelte` and `web/src/routes/participants/[id]/+page.svelte`.
  - Compose `EntityEditor` for the participant's editable fields; pass the participant's shifts section through `extras` (it uses `ShiftTable` with `onopen` — leave that intact).
  - Switch `participants/+page.svelte` to `rowHref`/`newHref` (it currently wires `onRowSave` + `detailHref`).
  - `npm run check`; manual verify. Commit `feat(web): participants edit page on EntityEditor with extras`.

---

## Task 8: Final gate

**Files:** none (verification only)

- [ ] **Step 1: Type-check, tests, build**

Run:
```bash
cd web && npm run check && npm test && npm run build
```
Expected: 0 errors / 0 warnings; all Vitest tests pass; SPA build succeeds (so the Go embed will compile).

- [ ] **Step 2: cgo-free binary still builds**

Run: `cd .. && CGO_ENABLED=0 go build ./cmd/tallyo`
Expected: builds clean (embeds the fresh `web/build`).

- [ ] **Step 3: Grep for dangling drawer references**

Run: `grep -rn "onRowSave\|detailHref\|drawerRow" web/src`
Expected: no matches (all call sites migrated; drawer fully removed).

- [ ] **Step 4: Final commit (if anything pending)**

```bash
git add -A && git commit -m "chore(web): finish route-based edit page migration" || echo "nothing to commit"
```

---

## Notes for the implementer

- **Shifts is intentionally excluded.** `ShiftTable`/`ShiftForm` keep the modal; do not touch the Shifts page (`web/src/routes/+page.svelte`).
- **`$app/state` vs `$app/stores`:** use `page.params.id` from `$app/state` (the modern rune API in current SvelteKit). If the repo is on an older Kit, fall back to `$app/stores` (`$page.params.id`). Check an existing `[id]` route (`invoices/[id]`) for which one the codebase uses and match it.
- **Don't widen autosave scope.** No queue panel, no blocking navigation guard — the spec explicitly rejected those.
- **Per-entity required fields:** copy the `required` constraints from each list page's old create form into the `validate` callback so create still enforces them.
- **Deliberate spec deviation:** the spec's prop table named a `store` prop "provides crud"; the plan passes `crud` directly (`crud={taxRates.crud}`) — cleaner and avoids coupling the editor to the whole store. Intentional; the plan is self-consistent on `crud`.
```
