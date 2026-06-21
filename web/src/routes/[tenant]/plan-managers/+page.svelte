<script lang="ts">
	import { onMount } from 'svelte';
	import { planManagers } from '$lib/stores/planManagers.svelte';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { PlanManager } from '$lib/api/types';

	onMount(() => {
		planManagers.ensureSubscribed();
		void planManagers.query({ page: 1, limit: 50 });
	});

	// DataTable column definitions. Keys match PlanManager JSON fields (and the
	// server allowlist), so one key drives filter, sort, display, and edit-page input kind.
	const columns: Column<PlanManager>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'email', label: 'Email', sortable: true, filter: 'text' },
		{ key: 'phone', label: 'Phone', sortable: true, filter: 'text' },
		{ key: 'address', label: 'Address', sortable: true, filter: 'text' }
	];

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
			rowHref={(r) => t(`/plan-managers/${r.id}`)}
			newHref={t('/plan-managers/new')}
		/>
	</section>
</div>
