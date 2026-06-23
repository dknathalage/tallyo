<script lang="ts">
	import { onMount } from 'svelte';
	import { recurring } from '$lib/stores/recurring.svelte';
	import { apiPost, tenantPath } from '$lib/api/client';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Zap from '@lucide/svelte/icons/zap';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { RecurringTemplate, Invoice } from '$lib/api/types';

	let rowError = $state<string | null>(null);
	let message = $state<string | null>(null);

	onMount(() => {
		recurring.ensureSubscribed();
		void recurring.query({ page: 1, limit: 50 });
	});

	// DataTable column definitions. Keys match RecurringTemplate JSON fields (and
	// the server allowlist), so one key drives filter, sort, and display. Row click
	// and the "+ New" button navigate to the edit / create routes.
	const columns: Column<RecurringTemplate>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'clientName', label: 'Client', sortable: true, filter: 'text' },
		{
			key: 'frequency',
			label: 'Frequency',
			sortable: true,
			filter: 'enum',
			values: ['weekly', 'monthly', 'quarterly']
		},
		{
			key: 'nextDue',
			label: 'Next due',
			sortable: true,
			filter: 'date',
			cell: (t) => (t.nextDue ? t.nextDue.slice(0, 10) : '—')
		},
		{
			key: 'taxRate',
			label: 'Tax rate',
			sortable: true,
			filter: 'number',
			cell: (t) => `${t.taxRate}%`
		},
		{
			// is_active is stored as 0/1; enum filter values are the integer strings.
			key: 'isActive',
			label: 'Status',
			sortable: true,
			filter: 'enum',
			values: ['1', '0'],
			cell: (t) => (t.isActive ? 'Active' : 'Inactive')
		}
	];

	async function generateNow(id: string): Promise<void> {
		rowError = null;
		message = null;
		try {
			const inv = await apiPost<Invoice>(tenantPath(`recurring/${id}/generate`), {});
			message = inv !== null ? 'Generated invoice ' + inv.number : 'Generated invoice.';
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to generate invoice.';
		}
	}

	// Table actions. Editing is on the detail route (/recurring/[id]); these bulk
	// actions loop over the selection. Mutations refresh via the SSE recurring
	// event re-running the active query.
	const rowActions: RowAction<RecurringTemplate>[] = [
		{
			label: 'Generate now',
			icon: Zap,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await generateNow(r.id); // bounded by selection
			}
		},
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await recurring.crud.remove(r.id); // bounded by selection
			}
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Recurring templates</h1>
			<p class="text-sm text-gray-500">Schedule invoices that generate on a recurring cadence.</p>
		</div>
	</section>

	<section>
		{#if recurring.error}
			<p class="mb-3 text-sm text-red-600">{recurring.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}
		{#if message}
			<p class="mb-3 text-sm text-green-700">{message}</p>
		{/if}

		<DataTable
			title="Recurring"
			{columns}
			store={recurring}
			{rowActions}
			rowHref={(r) => t(`/recurring/${r.id}`)}
			newHref={t('/recurring/new')}
		/>
	</section>
</div>
