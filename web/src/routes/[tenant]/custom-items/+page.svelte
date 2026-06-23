<script lang="ts">
	import { onMount } from 'svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { CustomItem } from '$lib/api/types';

	onMount(() => {
		customItems.ensureSubscribed();
		void customItems.query({ page: 1, limit: 50 });
	});

	// DataTable column definitions. Keys match CustomItem JSON fields (and the
	// server allowlist), so one key drives filter, sort, display, and edit-page input kind.
	const columns: Column<CustomItem>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{
			key: 'rate',
			label: 'Rate',
			sortable: true,
			filter: 'number',
			input: 'number',
			cell: (c) => c.rate.toFixed(2)
		},
		{ key: 'unit', label: 'Unit', sortable: true, filter: 'text' },
		{
			key: 'taxable',
			label: 'GST',
			sortable: true,
			input: 'checkbox',
			cell: (c) => (c.taxable ? 'Taxable' : '—')
		}
	];

	const rowActions: RowAction<CustomItem>[] = [
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await customItems.crud.remove(r.id); // bounded by selection
			}
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Custom items</h1>
			<p class="text-sm text-gray-500">
				Your own line items (e.g. travel, gap fees). Catalogue items come from the price list.
			</p>
		</div>
	</section>

	<section>
		{#if customItems.error}
			<p class="mb-3 text-sm text-red-600">{customItems.error}</p>
		{/if}

		<DataTable
			title="Custom items"
			{columns}
			store={customItems}
			{rowActions}
			rowHref={(r) => t(`/custom-items/${r.id}`)}
			newHref={t('/custom-items/new')}
		/>
	</section>
</div>
