<script lang="ts">
	import { onMount } from 'svelte';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { TaxRate, TaxRateInput } from '$lib/api/types';

	// New-tax-rate form fields.
	let newName = $state('');
	let newRate = $state(0);
	let newIsDefault = $state(false);
	let creating = $state(false);
	let formError = $state<string | null>(null);
	let showForm = $state(false);

	function openCreate(): void {
		resetNew();
		formError = null;
		showForm = true;
	}
	function cancelCreate(): void {
		resetNew();
		formError = null;
		showForm = false;
	}

	function resetNew(): void {
		newName = '';
		newRate = 0;
		newIsDefault = false;
	}

	onMount(() => {
		taxRates.ensureSubscribed();
		void taxRates.query({ page: 1, limit: 50 });
	});

	async function createTaxRate(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await taxRates.crud.create({
				name: newName,
				rate: newRate,
				isDefault: newIsDefault
			});
			resetNew();
			showForm = false;
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create tax rate.';
		} finally {
			creating = false;
		}
	}

	// DataTable column definitions. Keys match TaxRate JSON fields (and the server
	// allowlist), so one key drives filter, sort, display, and drawer edit.
	const columns: Column<TaxRate>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{
			key: 'rate',
			label: 'Rate',
			sortable: true,
			filter: 'number',
			cell: (t) => String(t.rate)
		},
		{
			key: 'isDefault',
			label: 'Default',
			sortable: true,
			cell: (t) => (t.isDefault ? 'Default' : '')
		}
	];

	// Map a (possibly drawer-edited) TaxRate back to its writable input.
	function toInput(t: TaxRate): TaxRateInput {
		return {
			name: t.name,
			rate: t.rate,
			isDefault: t.isDefault
		};
	}

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
			<h1 class="mb-1 text-xl font-semibold">Tax rates</h1>
			<p class="text-sm text-gray-500">Manage the tax rates applied to invoices.</p>
		</div>

		<Modal bind:open={showForm} title="New tax rate">
			<form class="flex flex-wrap items-end gap-3" onsubmit={createTaxRate}>
				<label class="flex-1">
					<span class="mb-1 block text-sm font-medium">Name</span>
					<input
						type="text"
						bind:value={newName}
						required
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="w-32">
					<span class="mb-1 block text-sm font-medium">Rate</span>
					<input
						type="number"
						step="0.01"
						bind:value={newRate}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="flex items-center gap-2 pb-2">
					<input type="checkbox" bind:checked={newIsDefault} class="h-4 w-4" />
					<span class="text-sm font-medium">Default</span>
				</label>
				{#if formError}
					<p class="w-full text-sm text-red-600">{formError}</p>
				{/if}
				<div class="flex w-full gap-2">
					<button
						type="submit"
						disabled={creating}
						class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
					>
						{creating ? 'Adding…' : 'Add tax rate'}
					</button>
					<button
						type="button"
						onclick={cancelCreate}
						class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50">Cancel</button
					>
				</div>
			</form>
		</Modal>
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
			onNew={openCreate}
			onRowSave={async (row) => {
				await taxRates.crud.update(row.id, toInput(row));
			}}
		/>
	</section>
</div>
