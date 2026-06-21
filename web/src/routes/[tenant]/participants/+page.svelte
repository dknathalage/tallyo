<script lang="ts">
	import { onMount } from 'svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { Participant } from '$lib/api/types';

	onMount(() => {
		participants.ensureSubscribed();
		void participants.query({ page: 1, limit: 50 });
	});

	// DataTable column definitions. Keys match Participant JSON fields (and the
	// server allowlist), so one key drives filter, sort, and display.
	const columns: Column<Participant>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'ndisNumber', label: 'NDIS #', sortable: true, filter: 'text' },
		{
			key: 'planStart',
			label: 'Plan start',
			sortable: true,
			filter: 'date',
			cell: (p) => (p.planStart ? p.planStart.slice(0, 10) : '—')
		},
		{
			key: 'planEnd',
			label: 'Plan end',
			sortable: true,
			filter: 'date',
			cell: (p) => (p.planEnd ? p.planEnd.slice(0, 10) : '—')
		},
		{
			key: 'mgmtType',
			label: 'Management',
			sortable: true,
			filter: 'enum',
			values: ['plan', 'self'],
			cell: (p) => (p.mgmtType === 'self' ? 'Self-managed' : 'Plan-managed')
		},
		{ key: 'planManagerName', label: 'Plan manager', sortable: true, filter: 'text' }
	];

	const rowActions: RowAction<Participant>[] = [
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await participants.crud.remove(r.id); // bounded by selection
			}
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Participants</h1>
			<p class="text-sm text-gray-500">
				NDIS participants you invoice — plan-managed or self-managed.
			</p>
		</div>
	</section>

	<section>
		{#if participants.error}
			<p class="mb-3 text-sm text-red-600">{participants.error}</p>
		{/if}

		<DataTable
			title="Participants"
			{columns}
			store={participants}
			{rowActions}
			rowHref={(p) => t(`/participants/${p.id}`)}
			newHref={t('/participants/new')}
		/>
	</section>
</div>
