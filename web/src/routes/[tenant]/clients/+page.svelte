<script lang="ts">
	import { onMount } from 'svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { Client } from '$lib/api/types';

	onMount(() => {
		clients.ensureSubscribed();
		void clients.query({ page: 1, limit: 50 });
	});

	// DataTable column definitions. Keys match Client JSON fields (and the
	// server allowlist), so one key drives filter, sort, and display.
	const columns: Column<Client>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{
			key: 'type',
			label: 'Type',
			sortable: true,
			filter: 'enum',
			values: ['standard', 'ndis'],
			cell: (p) => (p.type === 'ndis' ? 'NDIS' : 'Standard')
		},
		{ key: 'reference', label: 'Ref #', sortable: true, filter: 'text' },
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
		{ key: 'payerName', label: 'Payer', sortable: true, filter: 'text' }
	];

	const rowActions: RowAction<Client>[] = [
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await clients.crud.remove(r.id); // bounded by selection
			}
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Clients</h1>
			<p class="text-sm text-gray-500">
				NDIS clients you invoice — plan-managed or self-managed.
			</p>
		</div>
	</section>

	<section>
		{#if clients.error}
			<p class="mb-3 text-sm text-red-600">{clients.error}</p>
		{/if}

		<DataTable
			title="Clients"
			{columns}
			store={clients}
			{rowActions}
			rowHref={(p) => t(`/clients/${p.id}`)}
			newHref={t('/clients/new')}
		/>
	</section>
</div>
