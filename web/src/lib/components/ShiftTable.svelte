<script lang="ts">
	import { DataTable, type ColumnDef } from '@careswitch/svelte-data-table';
	import { dowDate, statusLabel, statusBadgeClass } from '$lib/shifts/format';
	import type { Shift, ShiftStatus } from '$lib/api/types';

	type Props = {
		shifts: Shift[];
		/** Maps a participant id → display name. */
		participantName: (id: number) => string;
		/** Row click — open the shift (edit / record). */
		onopen?: (shift: Shift) => void;
	};

	let { shifts, participantName, onopen }: Props = $props();

	// A flattened row carrying the derived participant name + a single tag string,
	// so the table can sort/search/filter on plain scalar columns.
	interface Row {
		id: number;
		date: string;
		participant: string;
		time: string;
		hours: number;
		km: number;
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
			time: s.startTime && s.endTime ? `${s.startTime}–${s.endTime}` : '',
			hours: s.hours,
			km: s.km,
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
		{ id: 'time', key: 'time', name: 'Time', sortable: false },
		{ id: 'hours', key: 'hours', name: 'Hrs', sortable: true },
		{ id: 'km', key: 'km', name: 'Km', sortable: true },
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
		table.baseRows = shifts.map(toRow);
	});

	// Per-column filters. Text columns (date, note) substring-match; the
	// participant/tag/status columns are exact-match dropdowns.
	let dateQuery = $state('');
	let noteQuery = $state('');
	let participantFilter = $state('');
	let tagFilter = $state('');
	let statusFilter = $state<'all' | ShiftStatus>('all');

	$effect(() => {
		if (dateQuery.trim() === '') table.clearFilter('date');
		else table.setFilter('date', [dateQuery]);
	});
	$effect(() => {
		if (noteQuery.trim() === '') table.clearFilter('note');
		else table.setFilter('note', [noteQuery]);
	});
	$effect(() => {
		if (participantFilter === '') table.clearFilter('participant');
		else table.setFilter('participant', [participantFilter]);
	});
	$effect(() => {
		if (tagFilter === '') table.clearFilter('tags');
		else table.setFilter('tags', [tagFilter]);
	});
	$effect(() => {
		if (statusFilter === 'all') table.clearFilter('status');
		else table.setFilter('status', [statusFilter]);
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
		return '↕';
	}
</script>

<div class="space-y-3">
	<div class="overflow-x-auto rounded border border-gray-200 bg-white">
		<table class="w-full text-sm">
			<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
				<tr>
					{#each table.columns as column (column.id)}
						{#if table.isSortable(column.id)}
							<th class="px-3 py-2 font-medium {column.id === 'hours' || column.id === 'km' ? 'text-right' : ''}">
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
							<th class="px-3 py-2 font-medium">{column.name}</th>
						{/if}
					{/each}
				</tr>
				<tr class="bg-white">
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
					<th></th>
					<th></th>
					<th></th>
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
						onclick={() => onopen?.(row.shift)}
					>
						<td class="px-3 py-2 whitespace-nowrap">{dowDate(row.date)}</td>
						<td class="px-3 py-2">{row.participant}</td>
						<td class="px-3 py-2 whitespace-nowrap text-gray-500">{row.time || '—'}</td>
						<td class="px-3 py-2 text-right">{row.hours || '—'}</td>
						<td class="px-3 py-2 text-right">{row.km || '—'}</td>
						<td class="max-w-[18rem] px-3 py-2 text-gray-600">
							{#if row.note}
								{row.note}
							{:else}
								<span class="text-gray-400">— not recorded —</span>
							{/if}
						</td>
						<td class="px-3 py-2">
							{#each row.shift.tags as tag (tag)}
								<span
									class="mr-1 mb-1 inline-block rounded bg-violet-50 px-1.5 py-0.5 text-xs font-medium text-violet-700 ring-1 ring-violet-200"
								>
									{tag}
								</span>
							{/each}
						</td>
						<td class="px-3 py-2">
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
						<td colspan="8" class="px-3 py-6 text-center text-gray-500">No matching shifts.</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>
