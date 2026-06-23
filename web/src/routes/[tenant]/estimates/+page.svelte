<script lang="ts">
	import { onMount } from 'svelte';
	import { estimates } from '$lib/stores/estimates.svelte';
	import { ApiError, apiPost, tenantPath } from '$lib/api/client';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import FileOutput from '@lucide/svelte/icons/file-output';
	import Copy from '@lucide/svelte/icons/copy';
	import Check from '@lucide/svelte/icons/check';
	import Ban from '@lucide/svelte/icons/ban';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { Estimate } from '$lib/api/types';

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	// DataTable column definitions. Keys match Estimate JSON fields (and the
	// server allowlist), so one key drives filter, sort and display. Row click and
	// the "+ New" button navigate to the detail / create routes (no inline drawer).
	const columns: Column<Estimate>[] = [
		{ key: 'number', label: 'Number', sortable: true, filter: 'text' },
		{ key: 'clientName', label: 'Client', sortable: true, filter: 'text' },
		{
			key: 'issueDate',
			label: 'Issued',
			sortable: true,
			filter: 'date',
			cell: (e) => (e.issueDate ? e.issueDate.slice(0, 10) : '—')
		},
		{
			key: 'total',
			label: 'Total',
			sortable: true,
			filter: 'number',
			cell: (e) => money(e.total)
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			filter: 'enum',
			values: ['draft', 'accepted', 'declined', 'converted']
		}
	];

	let rowError = $state<string | null>(null);

	// Table actions. The detail page (/estimates/[id]) owns the per-estimate
	// lifecycle; these bulk actions loop over the selection. Mutations refresh via
	// the SSE estimate event re-running the active query.
	const rowActions: RowAction<Estimate>[] = [
		{
			label: 'Convert to invoice',
			icon: FileOutput,
			bulk: true,
			run: async (rows) => {
				rowError = null;
				try {
					for (const r of rows) await apiPost(tenantPath(`estimates/${r.id}/convert`), {});
				} catch (err) {
					if (err instanceof ApiError) rowError = err.message;
					else rowError = err instanceof Error ? err.message : 'Failed to convert estimate.';
				}
			}
		},
		{
			label: 'Duplicate',
			icon: Copy,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await apiPost(tenantPath(`estimates/${r.id}/duplicate`), {});
			}
		},
		{
			label: 'Accept',
			icon: Check,
			bulk: true,
			run: async (rows) => {
				for (const r of rows)
					await apiPost(tenantPath(`estimates/${r.id}/status`), { status: 'accepted' });
			}
		},
		{
			label: 'Decline',
			icon: Ban,
			bulk: true,
			run: async (rows) => {
				for (const r of rows)
					await apiPost(tenantPath(`estimates/${r.id}/status`), { status: 'declined' });
			}
		},
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await estimates.crud.remove(r.id);
			}
		}
	];

	onMount(() => {
		estimates.ensureSubscribed();
		void estimates.query({ page: 1, limit: 50 });
	});
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Estimates</h1>
			<p class="text-sm text-gray-500">Quote NDIS work before invoicing.</p>
		</div>
	</section>

	<section>
		{#if estimates.error}
			<p class="mb-3 text-sm text-red-600">{estimates.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}

		<DataTable
			title="Estimates"
			{columns}
			store={estimates}
			{rowActions}
			rowHref={(e) => t(`/estimates/${e.id}`)}
			newHref={t('/estimates/new')}
		/>
	</section>
</div>
