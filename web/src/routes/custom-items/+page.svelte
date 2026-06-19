<script lang="ts">
	import { onMount } from 'svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { CustomItem, CustomItemInput } from '$lib/api/types';

	// New-item form fields.
	let newName = $state('');
	let newRate = $state(0);
	let newUnit = $state('');
	let newGstFree = $state(true);
	let creating = $state(false);
	let formError = $state<string | null>(null);
	let showCreate = $state(false);

	function resetNew(): void {
		newName = '';
		newRate = 0;
		newUnit = '';
		newGstFree = true;
	}

	function openCreate(): void {
		resetNew();
		formError = null;
		showCreate = true;
	}

	onMount(() => {
		customItems.ensureSubscribed();
		void customItems.query({ page: 1, limit: 50 });
	});

	async function createItem(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await customItems.crud.create({
				name: newName,
				rate: Number(newRate),
				unit: newUnit,
				gstFree: newGstFree,
				metadata: ''
			});
			resetNew();
			showCreate = false;
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create custom item.';
		} finally {
			creating = false;
		}
	}

	// DataTable column definitions. Keys match CustomItem JSON fields (and the
	// server allowlist), so one key drives filter, sort, display, and drawer edit.
	const columns: Column<CustomItem>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{
			key: 'rate',
			label: 'Rate',
			sortable: true,
			filter: 'number',
			cell: (c) => c.rate.toFixed(2)
		},
		{ key: 'unit', label: 'Unit', sortable: true, filter: 'text' },
		{
			key: 'gstFree',
			label: 'GST',
			sortable: true,
			cell: (c) => (c.gstFree ? 'GST-free' : '—')
		}
	];

	// Map a (possibly drawer-edited) CustomItem back to its writable input.
	function toInput(c: CustomItem): CustomItemInput {
		return {
			name: c.name,
			rate: Number(c.rate),
			unit: c.unit,
			gstFree: c.gstFree,
			metadata: c.metadata
		};
	}

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
				Your own non-NDIS line items (e.g. travel, gap fees). NDIS support items come from the
				Support catalogue.
			</p>
		</div>

		<Modal bind:open={showCreate} title="New item">
			<form class="grid grid-cols-2 gap-3" onsubmit={createItem}>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Name</span>
					<input
						type="text"
						bind:value={newName}
						required
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Rate</span>
					<input
						type="number"
						step="0.01"
						bind:value={newRate}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Unit</span>
					<input
						type="text"
						bind:value={newUnit}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="col-span-1 flex items-end gap-2">
					<input type="checkbox" bind:checked={newGstFree} class="h-4 w-4" />
					<span class="text-sm font-medium">GST-free</span>
				</label>
				{#if formError}
					<p class="col-span-2 text-sm text-red-600">{formError}</p>
				{/if}
				<div class="col-span-2 flex gap-2">
					<button
						type="submit"
						disabled={creating}
						class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
					>
						{creating ? 'Adding…' : 'Add item'}
					</button>
					<button
						type="button"
						onclick={() => (showCreate = false)}
						class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
					>
						Cancel
					</button>
				</div>
			</form>
		</Modal>
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
			onNew={openCreate}
			onRowSave={async (row) => {
				await customItems.crud.update(row.id, toInput(row));
			}}
		/>
	</section>
</div>
