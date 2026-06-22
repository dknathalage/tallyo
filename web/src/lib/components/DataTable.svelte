<script lang="ts" generics="T extends { id: string }">
	import type { ListParams } from '$lib/api/types';
	import type { Column, RowAction } from './datatable';
	import Plus from '@lucide/svelte/icons/plus';
	import Filter from '@lucide/svelte/icons/filter';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import X from '@lucide/svelte/icons/x';
	import { goto } from '$app/navigation';

	type Store = {
		rows: T[];
		total: number;
		loading: boolean;
		query: (p: ListParams) => Promise<void>;
	};

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

	// Per-column filter state. Reassigned (not mutated) so runes react.
	type FState =
		| { type: 'text'; value: string }
		| { type: 'enum'; vals: string[] }
		| { type: 'date'; from: string; to: string }
		| { type: 'number'; min: string; max: string };

	let sort = $state<string>('');
	let dir = $state<'asc' | 'desc'>('asc');
	let page = $state(1);
	let filters = $state<Record<string, FState>>({});
	let selected = $state<Set<string>>(new Set());
	let openMenu = $state<string | null>(null);
	let lastIdx = $state<number | null>(null);

	// Rows from the server; the component never filters/sorts locally.
	const rows = $derived(store.rows);
	const total = $derived(store.total);
	const pageCount = $derived(Math.max(1, Math.ceil(total / pageSize)));

	// ── Filters → params ──────────────────────────────────────────────────────
	function isActive(f: FState | undefined): boolean {
		if (!f) return false;
		if (f.type === 'text') return f.value !== '';
		if (f.type === 'enum') return f.vals.length > 0;
		if (f.type === 'date') return f.from !== '' || f.to !== '';
		return f.min !== '' || f.max !== '';
	}

	function buildFilterParams(): Record<string, string> {
		const out: Record<string, string> = {};
		for (const [key, f] of Object.entries(filters)) {
			if (!isActive(f)) continue;
			if (f.type === 'text') out[key] = f.value;
			else if (f.type === 'enum') out[key] = f.vals.join(',');
			else if (f.type === 'date') {
				if (f.from) out[key + '.from'] = f.from;
				if (f.to) out[key + '.to'] = f.to;
			} else {
				if (f.min) out[key + '.min'] = f.min;
				if (f.max) out[key + '.max'] = f.max;
			}
		}
		return out;
	}

	const params = $derived<ListParams>({
		sort: sort || undefined,
		dir,
		page,
		limit: pageSize,
		filters: buildFilterParams()
	});

	// Debounced server query whenever params change.
	$effect(() => {
		const p = params; // track
		const t = setTimeout(() => {
			void store.query(p);
		}, 200);
		return () => clearTimeout(t);
	});

	// Active-filter chips for the strip.
	const activeChips = $derived(
		columns
			.filter((c) => isActive(filters[c.key]))
			.map((c) => ({ key: c.key, label: c.label, text: chipText(filters[c.key]) }))
	);

	function chipText(f: FState): string {
		if (f.type === 'text') return `contains "${f.value}"`;
		if (f.type === 'enum') return f.vals.join(', ');
		if (f.type === 'date') return `${f.from || '…'} – ${f.to || '…'}`;
		return `${f.min || '…'} – ${f.max || '…'}`;
	}

	function clearFilter(key: string): void {
		const next = { ...filters };
		delete next[key];
		filters = next;
		page = 1;
	}
	function clearAllFilters(): void {
		filters = {};
		page = 1;
	}

	// ── Filter control mutators (reassign to react) ───────────────────────────
	function setText(key: string, value: string): void {
		filters = { ...filters, [key]: { type: 'text', value } };
		page = 1;
	}
	function toggleEnum(key: string, v: string): void {
		const cur = filters[key];
		const vals = cur && cur.type === 'enum' ? [...cur.vals] : [];
		const i = vals.indexOf(v);
		if (i >= 0) vals.splice(i, 1);
		else vals.push(v);
		filters = { ...filters, [key]: { type: 'enum', vals } };
		page = 1;
	}
	function setRange(key: string, kind: 'date' | 'number', which: string, value: string): void {
		const cur = filters[key];
		const base =
			kind === 'date'
				? cur && cur.type === 'date'
					? { ...cur }
					: { type: 'date' as const, from: '', to: '' }
				: cur && cur.type === 'number'
					? { ...cur }
					: { type: 'number' as const, min: '', max: '' };
		// which is 'from'|'to'|'min'|'max'
		(base as Record<string, unknown>)[which] = value;
		filters = { ...filters, [key]: base };
		page = 1;
	}

	function setSort(key: string, d: 'asc' | 'desc'): void {
		sort = key;
		dir = d;
	}

	// Typed getters for the menu controls (template can't narrow repeated index access).
	function textVal(key: string): string {
		const f = filters[key];
		return f?.type === 'text' ? f.value : '';
	}
	function enumHas(key: string, opt: string): boolean {
		const f = filters[key];
		return f?.type === 'enum' && f.vals.includes(opt);
	}
	function dateVal(key: string, which: 'from' | 'to'): string {
		const f = filters[key];
		return f?.type === 'date' ? f[which] : '';
	}
	function numVal(key: string, which: 'min' | 'max'): string {
		const f = filters[key];
		return f?.type === 'number' ? f[which] : '';
	}

	// ── Selection ─────────────────────────────────────────────────────────────
	function toggleRow(id: string, idx: number, shift: boolean): void {
		const next = new Set(selected);
		if (shift && lastIdx !== null) {
			const [a, b] = [lastIdx, idx].sort((x, y) => x - y);
			for (let i = a; i <= b; i++) next.add(rows[i].id);
		} else if (next.has(id)) next.delete(id);
		else next.add(id);
		selected = next;
		lastIdx = idx;
	}
	function toggleAll(): void {
		const allSel = rows.length > 0 && rows.every((r) => selected.has(r.id));
		const next = new Set(selected);
		if (allSel) rows.forEach((r) => next.delete(r.id));
		else rows.forEach((r) => next.add(r.id));
		selected = next;
	}
	function clearSelection(): void {
		selected = new Set();
	}
	const selectedRows = $derived(rows.filter((r) => selected.has(r.id)));
	const allChecked = $derived(rows.length > 0 && rows.every((r) => selected.has(r.id)));

	// ── Cell + display helpers ────────────────────────────────────────────────
	function cellText(col: Column<T>, row: T): string {
		if (col.cell) return col.cell(row);
		const v = (row as Record<string, unknown>)[col.key];
		return v === null || v === undefined || v === '' ? '—' : String(v);
	}

	// ── Global interactions ───────────────────────────────────────────────────
	function onWindowClick(e: MouseEvent): void {
		const t = e.target as HTMLElement;
		if (openMenu && !t.closest('.dt-menu') && !t.closest('.dt-head')) openMenu = null;
		if (selected.size && !t.closest('.dt-card')) clearSelection();
	}
	function onWindowKey(e: KeyboardEvent): void {
		const typing =
			e.target instanceof HTMLElement &&
			(e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT' || e.target.tagName === 'TEXTAREA');
		if (e.key === 'Escape') {
			if (openMenu) openMenu = null;
			else if (selected.size) clearSelection();
			return;
		}
		if ((e.metaKey || e.ctrlKey) && (e.key === 'a' || e.key === 'A') && !typing) {
			e.preventDefault();
			const next = new Set(selected);
			rows.forEach((r) => next.add(r.id));
			selected = next;
		}
	}
</script>

<svelte:window onclick={onWindowClick} onkeydown={onWindowKey} />

<div class="dt-card overflow-visible rounded border border-gray-200 bg-white">
	<!-- strip: title + filter chips, then New or selection actions -->
	<div class="flex min-h-[46px] items-center gap-2.5 border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs">
		<span class="shrink-0 text-sm font-semibold text-gray-900">{title}</span>

		{#each activeChips as chip (chip.key)}
			<span class="inline-flex items-center gap-1.5 rounded-full border border-gray-300 bg-white px-2.5 py-0.5">
				<span class="text-gray-400">{chip.label}</span>
				{chip.text}
				<button type="button" class="text-gray-400 hover:text-gray-900" onclick={() => clearFilter(chip.key)} aria-label="Clear {chip.label} filter">
					<X class="size-3" />
				</button>
			</span>
		{/each}
		{#if activeChips.length > 0}
			<button type="button" class="text-blue-600 hover:underline" onclick={clearAllFilters}>Clear filters</button>
		{/if}

		{#if selected.size > 0}
			<div class="ml-auto flex items-center gap-3.5">
				<span class="font-semibold text-gray-900">{selected.size} selected</span>
				{#each rowActions.filter((a) => a.bulk) as action (action.label)}
					<button
						type="button"
						class="inline-flex items-center gap-1.5 {action.danger ? 'text-red-700 hover:text-red-800' : 'text-gray-700 hover:text-gray-900'}"
						onclick={() => action.run(selectedRows)}
					>
						{#if action.icon}<action.icon class="size-3.5" />{/if}
						{action.label}
					</button>
				{/each}
				<button type="button" class="text-gray-400 hover:text-gray-900" onclick={clearSelection} aria-label="Clear selection">
					<X class="size-3.5" />
				</button>
			</div>
		{:else}
			<button
				type="button"
				class="ml-auto inline-flex items-center gap-1.5 rounded bg-gray-900 px-2.5 py-1.5 text-sm font-medium text-white hover:bg-gray-800"
				onclick={() => goto(newHref)}
			>
				<Plus class="size-3.5" /> New
			</button>
		{/if}
	</div>

	<!-- table -->
	<table class="w-full text-sm">
		<thead class="bg-gray-50 text-left text-gray-500">
			<tr>
				<th class="w-9 px-3 py-0 text-center">
					<input type="checkbox" class="size-3.5 accent-gray-900" checked={allChecked} onclick={toggleAll} aria-label="Select all" />
				</th>
				{#each columns as col (col.key)}
					<th class="relative p-0 font-medium">
						<button
							type="button"
							class="dt-head flex w-full items-center gap-1.5 px-3 py-2.5 text-left hover:bg-gray-100 hover:text-gray-900"
							onclick={(e) => {
								e.stopPropagation();
								openMenu = openMenu === col.key ? null : col.key;
							}}
						>
							<span>{col.label}</span>
							{#if sort === col.key}
								{#if dir === 'asc'}<ChevronUp class="size-3 text-gray-900" />{:else}<ChevronDown class="size-3 text-gray-900" />{/if}
							{/if}
							<Filter class="ml-auto size-3 {isActive(filters[col.key]) ? 'text-blue-600' : 'text-gray-300'}" />
						</button>

						{#if openMenu === col.key}
							<div class="dt-menu absolute right-2 top-10 z-30 w-56 rounded-lg border border-gray-300 bg-white p-2.5 text-left shadow-xl">
								<div class="mb-1.5 flex items-center justify-between text-[11px] uppercase tracking-wide text-gray-400">
									<span>{col.label}</span><span class="text-green-600 normal-case tracking-normal">● live</span>
								</div>

								{#if col.sortable}
									<div class="flex gap-1.5">
										<button type="button" class="flex-1 rounded border px-2 py-1.5 text-xs {sort === col.key && dir === 'asc' ? 'border-blue-200 bg-blue-50 font-medium text-blue-600' : 'border-gray-200 text-gray-700 hover:bg-gray-100'}" onclick={() => setSort(col.key, 'asc')}>▲ Asc</button>
										<button type="button" class="flex-1 rounded border px-2 py-1.5 text-xs {sort === col.key && dir === 'desc' ? 'border-blue-200 bg-blue-50 font-medium text-blue-600' : 'border-gray-200 text-gray-700 hover:bg-gray-100'}" onclick={() => setSort(col.key, 'desc')}>▼ Desc</button>
									</div>
									{#if col.filter}<div class="my-2 h-px bg-gray-100"></div>{/if}
								{/if}

								{#if col.filter === 'text'}
									<div class="mb-1.5 text-[11px] uppercase tracking-wide text-gray-400">Search</div>
									<input
										type="text"
										placeholder="contains…"
										value={textVal(col.key)}
										oninput={(e) => setText(col.key, e.currentTarget.value)}
										class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
									/>
								{:else if col.filter === 'enum'}
									<div class="mb-1.5 text-[11px] uppercase tracking-wide text-gray-400">Filter</div>
									{#each col.values ?? [] as opt (opt)}
										<label class="flex cursor-pointer items-center gap-2 py-0.5 text-sm">
											<input
												type="checkbox"
												class="size-3.5 accent-gray-900"
												checked={enumHas(col.key, opt)}
												onchange={() => toggleEnum(col.key, opt)}
											/>
											{opt}
										</label>
									{/each}
								{:else if col.filter === 'date'}
									<div class="mb-1.5 text-[11px] uppercase tracking-wide text-gray-400">Range</div>
									<input type="date" value={dateVal(col.key, 'from')} onchange={(e) => setRange(col.key, 'date', 'from', e.currentTarget.value)} class="mb-1.5 w-full rounded border border-gray-300 px-2 py-1.5 text-sm" />
									<input type="date" value={dateVal(col.key, 'to')} onchange={(e) => setRange(col.key, 'date', 'to', e.currentTarget.value)} class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm" />
								{:else if col.filter === 'number'}
									<div class="mb-1.5 text-[11px] uppercase tracking-wide text-gray-400">Range</div>
									<input type="number" placeholder="min" value={numVal(col.key, 'min')} oninput={(e) => setRange(col.key, 'number', 'min', e.currentTarget.value)} class="mb-1.5 w-full rounded border border-gray-300 px-2 py-1.5 text-sm" />
									<input type="number" placeholder="max" value={numVal(col.key, 'max')} oninput={(e) => setRange(col.key, 'number', 'max', e.currentTarget.value)} class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm" />
								{/if}
							</div>
						{/if}
					</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each rows as row, idx (row.id)}
				<tr
					class="cursor-pointer border-t border-gray-100 {selected.has(row.id) ? 'bg-blue-50 hover:bg-blue-100' : 'hover:bg-gray-50'}"
					onclick={() => goto(rowHref(row))}
				>
					<td class="w-9 px-3 py-2 text-center">
						<input
							type="checkbox"
							class="size-3.5 accent-gray-900"
							checked={selected.has(row.id)}
							onclick={(e) => {
								e.stopPropagation();
								toggleRow(row.id, idx, e.shiftKey);
							}}
							aria-label="Select row"
						/>
					</td>
					{#each columns as col (col.key)}
						<td class="px-3 py-2 {col.key === columns[0].key ? 'font-medium text-gray-900' : 'text-gray-600'}">
							{#if col.filter === 'enum'}
								<span class="rounded-full bg-gray-200 px-2 py-0.5 text-xs text-gray-700">{cellText(col, row)}</span>
							{:else}
								{cellText(col, row)}
							{/if}
						</td>
					{/each}
				</tr>
			{:else}
				<tr>
					<td colspan={columns.length + 1} class="px-3 py-7 text-center text-gray-400">
						{store.loading ? 'Loading…' : 'No matching rows.'}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>

	<!-- footer -->
	<div class="flex items-center justify-between border-t border-gray-100 px-3.5 py-2.5 text-xs text-gray-500">
		<span>{total} {total === 1 ? 'row' : 'rows'}</span>
		<span class="flex items-center gap-2">
			Page {page} of {pageCount}
			<button type="button" class="rounded border border-gray-300 px-2 py-0.5 disabled:opacity-40" disabled={page <= 1} onclick={() => (page = page - 1)}>‹</button>
			<button type="button" class="rounded border border-gray-300 px-2 py-0.5 disabled:opacity-40" disabled={page >= pageCount} onclick={() => (page = page + 1)}>›</button>
		</span>
	</div>
</div>
