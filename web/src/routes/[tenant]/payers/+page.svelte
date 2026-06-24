<script lang="ts">
	import { onMount } from 'svelte';
	import { payers } from '$lib/stores/payers.svelte';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import CreateModal from '$lib/components/CreateModal.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { Payer, PayerInput } from '$lib/api/types';

	onMount(() => {
		payers.ensureSubscribed();
		void payers.query({ page: 1, limit: 50 });
	});

	let createOpen = $state(false);

	function toInput(p: Payer): PayerInput {
		return { name: p.name, email: p.email, phone: p.phone, address: p.address, metadata: p.metadata ?? '' };
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

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
			<h1 class="mb-1 text-2xl font-semibold tracking-tight">Payers</h1>
			<p class="text-sm text-gray-500">
				Third parties you invoice on behalf of clients (e.g. a plan manager or funding body).
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
			onnew={() => (createOpen = true)}
		/>
	</section>
</div>

<CreateModal
	title="payer"
	{columns}
	create={payers.crud.create}
	{toInput}
	{validate}
	bind:open={createOpen}
	onsaved={() => payers.query({ page: 1, limit: 50 })}
/>
