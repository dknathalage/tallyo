<script lang="ts">
	import { onMount } from 'svelte';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import CreateModal from '$lib/components/CreateModal.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { TaxRate, TaxRateInput } from '$lib/api/types';

	onMount(() => {
		taxRates.ensureSubscribed();
		void taxRates.query({ page: 1, limit: 50 });
	});

	let createOpen = $state(false);

	function toInput(r: TaxRate): TaxRateInput {
		return { name: r.name, rate: r.rate, isDefault: r.isDefault };
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	// DataTable column definitions. Keys match TaxRate JSON fields (and the server
	// allowlist), so one key drives filter, sort, display, and edit-page input kind.
	const columns: Column<TaxRate>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'rate', label: 'Rate', sortable: true, filter: 'number', cell: (t) => String(t.rate) },
		{
			key: 'isDefault',
			label: 'Default',
			sortable: true,
			input: 'checkbox',
			cell: (t) => (t.isDefault ? 'Default' : '')
		}
	];

	const rowActions: RowAction<TaxRate>[] = [
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await taxRates.crud.remove(r.id); // bounded by selection
			}
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-2xl font-semibold tracking-tight">Tax rates</h1>
			<p class="text-sm text-gray-500">Manage the tax rates applied to invoices.</p>
		</div>
	</section>

	<section>
		{#if taxRates.error}
			<p class="mb-3 text-sm text-red-600">{taxRates.error}</p>
		{/if}

		<DataTable
			title="Tax rates"
			{columns}
			store={taxRates}
			{rowActions}
			rowHref={(r) => t(`/tax-rates/${r.id}`)}
			onnew={() => (createOpen = true)}
		/>
	</section>
</div>

<CreateModal
	title="tax rate"
	{columns}
	create={taxRates.crud.create}
	{toInput}
	{validate}
	bind:open={createOpen}
	onsaved={() => taxRates.query({ page: 1, limit: 50 })}
/>
