<script lang="ts">
	import { onMount } from 'svelte';
	import { payers } from '$lib/stores/payers.svelte';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { Payer } from '$lib/api/types';

	onMount(() => {
		payers.ensureSubscribed();
		void payers.query({ page: 1, limit: 50 });
	});

	// DataTable column definitions. Keys match Payer JSON fields (and the
	// server allowlist), so one key drives filter, sort, display, and edit-page input kind.
	const columns: Column<Payer>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'email', label: 'Email', sortable: true, filter: 'text' },
		{ key: 'phone', label: 'Phone', sortable: true, filter: 'text' },
		{ key: 'address', label: 'Address', sortable: true, filter: 'text' }
	];

	const rowActions: RowAction<Payer>[] = [
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await payers.crud.remove(r.id); // bounded by selection
			}
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Payers</h1>
			<p class="text-sm text-gray-500">
				NDIS plan-management organisations you invoice on behalf of clients.
			</p>
		</div>
	</section>

	<section>
		{#if payers.error}
			<p class="mb-3 text-sm text-red-600">{payers.error}</p>
		{/if}

		<DataTable
			title="Payers"
			{columns}
			store={payers}
			{rowActions}
			rowHref={(r) => t(`/payers/${r.id}`)}
			newHref={t('/payers/new')}
		/>
	</section>
</div>
