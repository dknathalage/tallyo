<script lang="ts">
	import { onMount } from 'svelte';
	import { planManagers } from '$lib/stores/planManagers.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { PlanManager, PlanManagerInput } from '$lib/api/types';

	// New-plan-manager form fields.
	let newName = $state('');
	let newEmail = $state('');
	let newPhone = $state('');
	let newAddress = $state('');
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

	onMount(() => {
		planManagers.ensureSubscribed();
		void planManagers.query({ page: 1, limit: 50 });
	});

	function resetNew(): void {
		newName = '';
		newEmail = '';
		newPhone = '';
		newAddress = '';
	}

	async function createManager(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await planManagers.crud.create({
				name: newName,
				email: newEmail,
				phone: newPhone,
				address: newAddress,
				metadata: ''
			});
			resetNew();
			showForm = false;
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create plan manager.';
		} finally {
			creating = false;
		}
	}

	// DataTable column definitions. Keys match PlanManager JSON fields (and the
	// server allowlist), so one key drives filter, sort, display, and drawer edit.
	const columns: Column<PlanManager>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'email', label: 'Email', sortable: true, filter: 'text' },
		{ key: 'phone', label: 'Phone', sortable: true, filter: 'text' },
		{ key: 'address', label: 'Address', sortable: true, filter: 'text' }
	];

	// Map a (possibly drawer-edited) PlanManager back to its writable input.
	function toInput(p: PlanManager): PlanManagerInput {
		return {
			name: p.name,
			email: p.email,
			phone: p.phone,
			address: p.address,
			metadata: p.metadata
		};
	}

	const rowActions: RowAction<PlanManager>[] = [
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await planManagers.crud.remove(r.id); // bounded by selection
			}
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Plan managers</h1>
			<p class="text-sm text-gray-500">
				NDIS plan-management organisations you invoice on behalf of participants.
			</p>
		</div>

		<Modal bind:open={showForm} title="New plan manager">
			<form class="grid grid-cols-2 gap-3" onsubmit={createManager}>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Name</span>
					<input type="text" bind:value={newName} required class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Email</span>
					<input type="email" bind:value={newEmail} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Phone</span>
					<input type="text" bind:value={newPhone} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Address</span>
					<input type="text" bind:value={newAddress} class="w-full rounded border border-gray-300 px-3 py-2 text-sm" />
				</label>
				{#if formError}
					<p class="col-span-2 text-sm text-red-600">{formError}</p>
				{/if}
				<div class="col-span-2 flex gap-2">
					<button type="submit" disabled={creating} class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">
						{creating ? 'Adding…' : 'Add plan manager'}
					</button>
					<button type="button" onclick={cancelCreate} class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50">Cancel</button>
				</div>
			</form>
		</Modal>
	</section>

	<section>
		{#if planManagers.error}
			<p class="mb-3 text-sm text-red-600">{planManagers.error}</p>
		{/if}

		<DataTable
			title="Plan managers"
			{columns}
			store={planManagers}
			{rowActions}
			onNew={openCreate}
			onRowSave={async (row) => {
				await planManagers.crud.update(row.id, toInput(row));
			}}
		/>
	</section>
</div>
