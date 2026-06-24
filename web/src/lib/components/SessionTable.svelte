<script lang="ts">
	import { untrack } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { DataTable, type ColumnDef } from '@careswitch/svelte-data-table';
	import { dowDate, statusLabel } from '$lib/sessions/format';
	import Badge from '$lib/components/Badge.svelte';
	import Button from '$lib/components/Button.svelte';
	import type { Session, SessionStatus } from '$lib/api/types';

	// Lifecycle status → Badge tone. Mirrors the session palette
	// (scheduled=amber, recorded=blue, drafted=slate, sent=brand/teal, paid=green).
	function statusTone(status: string): 'amber' | 'blue' | 'slate' | 'brand' | 'green' | 'gray' {
		switch (status) {
			case 'scheduled':
				return 'amber';
			case 'recorded':
				return 'blue';
			case 'drafted':
				return 'slate';
			case 'sent':
				return 'brand';
			case 'paid':
				return 'green';
			default:
				return 'gray';
		}
	}

	type Props = {
		sessions: Session[];
		/** Maps a client uuid → display name. */
		clientName: (id: string) => string;
		/** Row click — open the session (edit / record). */
		onopen?: (session: Session) => void;
		/** Bulk delete — called with the selected session uuids. */
		ondelete?: (ids: string[]) => void | Promise<void>;
	};

	let { sessions, clientName, onopen, ondelete }: Props = $props();

	// A flattened row carrying the derived client name + a single tag string,
	// so the table can sort/search/filter on plain scalar columns.
	interface Row {
		id: string;
		date: string;
		client: string;
		note: string;
		tags: string;
		status: SessionStatus;
		session: Session;
	}

	function toRow(s: Session): Row {
		return {
			id: s.id,
			date: s.serviceDate,
			client: clientName(s.clientId),
			note: s.note,
			tags: s.tags.join(', '),
			status: s.status,
			session: s
		};
	}

	const STATUSES: SessionStatus[] = ['scheduled', 'recorded', 'drafted', 'sent', 'paid'];

	// Case-insensitive substring match, shared by the free-text column filters.
	function textFilter(value: string, filterValue: string): boolean {
		return String(value ?? '')
			.toLowerCase()
			.includes(String(filterValue ?? '').toLowerCase());
	}

	const columns: ColumnDef<Row>[] = [
		{ id: 'date', key: 'date', name: 'Date', sortable: true, filter: textFilter },
		{
			id: 'client',
			key: 'client',
			name: 'Client',
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
			filter: (value: SessionStatus, filterValue: SessionStatus) => value === filterValue
		}
	];

	// Initialised empty; the effect below seeds + keeps baseRows in sync with the
	// reactive `sessions` prop (reading it in the constructor would only capture the
	// initial value).
	const table = new DataTable<Row>({
		data: [],
		columns,
		initialSort: 'date',
		initialSortDirection: 'desc'
	});

	$effect(() => {
		const rows = sessions.map(toRow);
		// setFilter/clearFilter below (and baseRows) read #filterState while writing
		// it; running the table mutations untracked keeps these effects from
		// subscribing to that write and looping (effect_update_depth_exceeded).
		untrack(() => {
			table.baseRows = rows;
		});
	});

	// Per-column filters. Text columns (date, note) substring-match; the
	// client/tag/status columns are exact-match dropdowns.
	let dateQuery = $state('');
	let noteQuery = $state('');
	let clientFilter = $state('');
	let tagFilter = $state('');
	let statusFilter = $state<'all' | SessionStatus>('all');

	$effect(() => {
		const q = dateQuery;
		untrack(() => (q.trim() === '' ? table.clearFilter('date') : table.setFilter('date', [q])));
	});
	$effect(() => {
		const q = noteQuery;
		untrack(() => (q.trim() === '' ? table.clearFilter('note') : table.setFilter('note', [q])));
	});
	$effect(() => {
		const q = clientFilter;
		untrack(() =>
			q === '' ? table.clearFilter('client') : table.setFilter('client', [q])
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

	// Distinct client names + tag codes for the dropdowns (from current data).
	const clientNames = $derived(
		Array.from(new Set(sessions.map((s) => clientName(s.clientId)))).sort()
	);
	const tagOptions = $derived(Array.from(new Set(sessions.flatMap((s) => s.tags))).sort());

	function sortIndicator(columnId: string): string {
		const dir = table.getSortState(columnId);
		if (dir === 'asc') return '▲';
		if (dir === 'desc') return '▼';
		return ''; // no glyph until the column is the active sort
	}

	// Row selection for bulk actions. Keyed by session id so it survives re-sorts
	// and filter changes; ids that filter out of view simply aren't acted on.
	const selected = new SvelteSet<string>();

	const visibleIds = $derived(table.rows.map((r) => r.id));
	const allVisibleSelected = $derived(
		visibleIds.length > 0 && visibleIds.every((id) => selected.has(id))
	);

	function toggleRow(id: string): void {
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
		if (!confirm(`Delete ${ids.length} session${ids.length === 1 ? '' : 's'}? This cannot be undone.`))
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
			class="flex items-center gap-3 rounded-lg border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm"
		>
			<span class="font-medium text-gray-700"><span class="font-mono tabular-nums">{selected.size}</span> selected</span>
			<Button variant="danger" size="sm" onclick={deleteSelected} loading={deleting} disabled={deleting}>
				{deleting ? 'Deleting…' : 'Delete'}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => selected.clear()}>
				Clear
			</Button>
		</div>
	{/if}
	<div class="max-h-80 overflow-auto rounded-xl border border-gray-200 bg-white shadow-sm">
		<table class="w-full text-sm">
			<thead class="sticky top-0 z-10 border-b border-gray-200 bg-gray-50 text-left text-gray-500">
				<tr>
					<th class="px-3 py-1.5">
						<input
							type="checkbox"
							checked={allVisibleSelected}
							onchange={toggleAllVisible}
							aria-label="Select all sessions"
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
							class="w-full rounded-lg border border-gray-300 px-2 py-1 text-xs font-normal"
						/>
					</th>
					<th class="px-3 py-1.5">
						<select
							bind:value={clientFilter}
							aria-label="Filter by client"
							class="w-full rounded-lg border border-gray-300 px-1 py-1 text-xs font-normal"
						>
							<option value="">All</option>
							{#each clientNames as n (n)}
								<option value={n}>{n}</option>
							{/each}
						</select>
					</th>
					<th class="px-3 py-1.5">
						<input
							bind:value={noteQuery}
							placeholder="search…"
							aria-label="Search notes"
							class="w-full rounded-lg border border-gray-300 px-2 py-1 text-xs font-normal"
						/>
					</th>
					<th class="px-3 py-1.5">
						<select
							bind:value={tagFilter}
							aria-label="Filter by tag"
							class="w-full rounded-lg border border-gray-300 px-1 py-1 text-xs font-normal"
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
							class="w-full rounded-lg border border-gray-300 px-1 py-1 text-xs font-normal"
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
						class:bg-brand-50={selected.has(row.id)}
						onclick={() => onopen?.(row.session)}
					>
						<td class="px-3 py-1.5">
							<input
								type="checkbox"
								checked={selected.has(row.id)}
								onclick={(e) => e.stopPropagation()}
								onchange={() => toggleRow(row.id)}
								aria-label="Select session"
								class="align-middle"
							/>
						</td>
						<td class="px-3 py-1.5 whitespace-nowrap font-mono tabular-nums">{dowDate(row.date)}</td>
						<td class="px-3 py-1.5">{row.client}</td>
						<td class="max-w-[18rem] px-3 py-1.5 text-gray-600">
							{#if row.note}
								{row.note}
							{:else}
								<span class="text-gray-400">— not recorded —</span>
							{/if}
						</td>
						<td class="px-3 py-1.5">
							{#each row.session.tags as tag (tag)}
								<span class="mr-1 mb-1 inline-block">
									<Badge tone="gray">{tag}</Badge>
								</span>
							{/each}
						</td>
						<td class="px-3 py-1.5">
							<Badge tone={statusTone(row.status)} class="whitespace-nowrap">
								{statusLabel(row.status)}
							</Badge>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="9" class="px-3 py-6 text-center text-gray-500">No matching sessions.</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>
