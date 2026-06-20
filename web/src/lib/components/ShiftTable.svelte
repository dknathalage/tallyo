<script lang="ts">
	import { untrack } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { DataTable, type ColumnDef } from '@careswitch/svelte-data-table';
	import { dowDate, statusLabel, statusBadgeClass } from '$lib/shifts/format';
	import type { Shift, ShiftStatus } from '$lib/api/types';

	type Props = {
		shifts: Shift[];
		/** Maps a participant id → display name. */
		participantName: (id: number) => string;
		/** Row click — open the shift (edit / record). */
		onopen?: (shift: Shift) => void;
		/** Bulk delete — called with the selected shift ids. */
		ondelete?: (ids: number[]) => void | Promise<void>;
	};

	let { shifts, participantName, onopen, ondelete }: Props = $props();

	// A flattened row carrying the derived participant name + a single tag string,
	// so the table can sort/search/filter on plain scalar columns.
	interface Row {
		id: number;
		date: string;
		participant: string;
		note: string;
		tags: string;
		status: ShiftStatus;
		shift: Shift;
	}

	function toRow(s: Shift): Row {
		return {
			id: s.id,
			date: s.serviceDate,
			participant: participantName(s.participantId),
			note: s.note,
			tags: s.tags.join(', '),
			status: s.status,
			shift: s
		};
	}

	const STATUSES: ShiftStatus[] = ['scheduled', 'recorded', 'drafted', 'sent', 'paid'];

	// Case-insensitive substring match, shared by the free-text column filters.
	function textFilter(value: string, filterValue: string): boolean {
		return String(value ?? '')
			.toLowerCase()
			.includes(String(filterValue ?? '').toLowerCase());
	}

	const columns: ColumnDef<Row>[] = [
		{ id: 'date', key: 'date', name: 'Date', sortable: true, filter: textFilter },
		{
			id: 'participant',
			key: 'participant',
			name: 'Participant',
			sortable: true,
			filter: (value: string, filterValue: string) => value === filterValue
		},
		{ id: 'note', key: 'note', name: 'Note', sortable: false, filter: textFilter },
		{ id: 'tags', key: 'tags', name: 'Tags', sortable: false, filter: textFilter },
		{
			id: 'status',
			key: 'status',
			name: 'Status',
			sortable: true,
			filter: (value: ShiftStatus, filterValue: ShiftStatus) => value === filterValue
		}
	];

	// Initialised empty; the effect below seeds + keeps baseRows in sync with the
	// reactive `shifts` prop (reading it in the constructor would only capture the
	// initial value).
	const table = new DataTable<Row>({
		data: [],
		columns,
		initialSort: 'date',
		initialSortDirection: 'desc'
	});

	$effect(() => {
		const rows = shifts.map(toRow);
		// setFilter/clearFilter below (and baseRows) read #filterState while writing
		// it; running the table mutations untracked keeps these effects from
		// subscribing to that write and looping (effect_update_depth_exceeded).
		untrack(() => {
			table.baseRows = rows;
		});
	});

	// Per-column filters. Text columns (date, note) substring-match; the
	// participant/tag/status columns are exact-match dropdowns.
	let dateQuery = $state('');
	let noteQuery = $state('');
	let participantFilter = $state('');
	let tagFilter = $state('');
	let statusFilter = $state<'all' | ShiftStatus>('all');

	$effect(() => {
		const q = dateQuery;
		untrack(() => (q.trim() === '' ? table.clearFilter('date') : table.setFilter('date', [q])));
	});
	$effect(() => {
		const q = noteQuery;
		untrack(() => (q.trim() === '' ? table.clearFilter('note') : table.setFilter('note', [q])));
	});
	$effect(() => {
		const q = participantFilter;
		untrack(() =>
			q === '' ? table.clearFilter('participant') : table.setFilter('participant', [q])
		);
	});
	$effect(() => {
		const q = tagFilter;
		untrack(() => (q === '' ? table.clearFilter('tags') : table.setFilter('tags', [q])));
	});
	$effect(() => {
		const q = statusFilter;
		untrack(() => (q === 'all' ? table.clearFilter('status') : table.setFilter('status', [q])));
	});

	// Distinct participant names + tag codes for the dropdowns (from current data).
	const participantNames = $derived(
		Array.from(new Set(shifts.map((s) => participantName(s.participantId)))).sort()
	);
	const tagOptions = $derived(Array.from(new Set(shifts.flatMap((s) => s.tags))).sort());

	function sortIndicator(columnId: string): string {
		const dir = table.getSortState(columnId);
		if (dir === 'asc') return '▲';
		if (dir === 'desc') return '▼';
		return ''; // no glyph until the column is the active sort
	}

	// Row selection for bulk actions. Keyed by shift id so it survives re-sorts
	// and filter changes; ids that filter out of view simply aren't acted on.
	const selected = new SvelteSet<number>();

	const visibleIds = $derived(table.rows.map((r) => r.id));
	const allVisibleSelected = $derived(
		visibleIds.length > 0 && visibleIds.every((id) => selected.has(id))
	);

	function toggleRow(id: number): void {
		if (selected.has(id)) selected.delete(id);
		else selected.add(id);
	}

	function toggleAllVisible(): void {
		if (allVisibleSelected) {
			for (const id of visibleIds) selected.delete(id);
		} else {
			for (const id of visibleIds) selected.add(id);
		}
	}

	let deleting = $state(false);

	async function deleteSelected(): Promise<void> {
		const ids = Array.from(selected);
		if (ids.length === 0 || deleting) return;
		if (!confirm(`Delete ${ids.length} shift${ids.length === 1 ? '' : 's'}? This cannot be undone.`))
			return;
		deleting = true;
		try {
			await ondelete?.(ids);
			selected.clear();
		} finally {
			deleting = false;
		}
	}
</script>

<div class="space-y-3">
	{#if selected.size > 0}
		<div
			class="flex items-center gap-3 rounded border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm"
		>
			<span class="font-medium text-gray-700">{selected.size} selected</span>
			<button
				type="button"
				onclick={deleteSelected}
				disabled={deleting}
				class="rounded bg-red-600 px-2.5 py-1 text-xs font-semibold text-white hover:bg-red-700 disabled:opacity-50"
			>
				{deleting ? 'Deleting…' : 'Delete'}
			</button>
			<button
				type="button"
				onclick={() => selected.clear()}
				class="text-xs text-gray-500 hover:text-gray-900"
			>
				Clear
			</button>
		</div>
	{/if}
	<div class="max-h-80 overflow-auto rounded border border-gray-200 bg-white">
		<table class="w-full text-sm">
			<thead class="sticky top-0 z-10 border-b border-gray-200 bg-gray-50 text-left text-gray-500">
				<tr>
					<th class="px-3 py-1.5">
						<input
							type="checkbox"
							checked={allVisibleSelected}
							onchange={toggleAllVisible}
							aria-label="Select all shifts"
							class="align-middle"
						/>
					</th>
					{#each table.columns as column (column.id)}
						{#if table.isSortable(column.id)}
							<th class="px-3 py-1.5 font-medium {column.id === 'hours' || column.id === 'km' ? 'text-right' : ''}">
								<button
									type="button"
									onclick={() => table.toggleSort(column.id)}
									class="inline-flex items-center gap-1 font-medium hover:text-gray-900"
								>
									{column.name}
									<span class="text-xs text-gray-400">{sortIndicator(column.id)}</span>
								</button>
							</th>
						{:else}
							<th class="px-3 py-1.5 font-medium">{column.name}</th>
						{/if}
					{/each}
				</tr>
				<tr class="bg-white">
					<th></th>
					<th class="px-3 py-1.5">
						<input
							bind:value={dateQuery}
							placeholder="filter…"
							aria-label="Filter by date"
							class="w-full rounded border border-gray-300 px-2 py-1 text-xs font-normal"
						/>
					</th>
					<th class="px-3 py-1.5">
						<select
							bind:value={participantFilter}
							aria-label="Filter by participant"
							class="w-full rounded border border-gray-300 px-1 py-1 text-xs font-normal"
						>
							<option value="">All</option>
							{#each participantNames as n (n)}
								<option value={n}>{n}</option>
							{/each}
						</select>
					</th>
					<th class="px-3 py-1.5">
						<input
							bind:value={noteQuery}
							placeholder="search…"
							aria-label="Search notes"
							class="w-full rounded border border-gray-300 px-2 py-1 text-xs font-normal"
						/>
					</th>
					<th class="px-3 py-1.5">
						<select
							bind:value={tagFilter}
							aria-label="Filter by tag"
							class="w-full rounded border border-gray-300 px-1 py-1 text-xs font-normal"
						>
							<option value="">All</option>
							{#each tagOptions as t (t)}
								<option value={t}>{t}</option>
							{/each}
						</select>
					</th>
					<th class="px-3 py-1.5">
						<select
							bind:value={statusFilter}
							aria-label="Filter by status"
							class="w-full rounded border border-gray-300 px-1 py-1 text-xs font-normal"
						>
							<option value="all">All</option>
							{#each STATUSES as s (s)}
								<option value={s}>{statusLabel(s)}</option>
							{/each}
						</select>
					</th>
				</tr>
			</thead>
			<tbody>
				{#each table.rows as row (row.id)}
					<tr
						class="cursor-pointer border-b border-gray-100 last:border-0 hover:bg-gray-50"
						class:bg-blue-50={selected.has(row.id)}
						onclick={() => onopen?.(row.shift)}
					>
						<td class="px-3 py-1.5">
							<input
								type="checkbox"
								checked={selected.has(row.id)}
								onclick={(e) => e.stopPropagation()}
								onchange={() => toggleRow(row.id)}
								aria-label="Select shift"
								class="align-middle"
							/>
						</td>
						<td class="px-3 py-1.5 whitespace-nowrap">{dowDate(row.date)}</td>
						<td class="px-3 py-1.5">{row.participant}</td>
						<td class="max-w-[18rem] px-3 py-1.5 text-gray-600">
							{#if row.note}
								{row.note}
							{:else}
								<span class="text-gray-400">— not recorded —</span>
							{/if}
						</td>
						<td class="px-3 py-1.5">
							{#each row.shift.tags as tag (tag)}
								<span
									class="mr-1 mb-1 inline-block rounded bg-gray-100 px-1.5 py-0.5 text-xs font-medium text-gray-700 ring-1 ring-gray-200"
								>
									{tag}
								</span>
							{/each}
						</td>
						<td class="px-3 py-1.5">
							<span
								class="inline-block rounded px-2 py-0.5 text-xs font-semibold whitespace-nowrap {statusBadgeClass(
									row.status
								)}"
							>
								{statusLabel(row.status)}
							</span>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="9" class="px-3 py-6 text-center text-gray-500">No matching shifts.</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>
