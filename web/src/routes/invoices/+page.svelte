<script lang="ts">
	import { onMount } from 'svelte';
	import { invoices } from '$lib/stores/invoices.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column } from '$lib/components/datatable';
	import type { Invoice, InvoiceStatus } from '$lib/api/types';

	const STATUSES: InvoiceStatus[] = ['draft', 'sent', 'overdue', 'paid'];

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	// Read-only payment-status label for the list. The detail page owns the
	// actual payment workflow; here we only summarise paid vs total.
	function paymentLabel(total: number, paid: number): string {
		const t = Number.isFinite(total) ? total : 0;
		const p = Number.isFinite(paid) ? paid : 0;
		if (p >= t && t > 0) return 'Paid';
		if (p > 0) return 'Partial';
		return 'Unpaid';
	}

	onMount(() => {
		invoices.ensureSubscribed();
		void invoices.query({ page: 1, limit: 50 });
	});

	// DataTable column definitions. Keys match Invoice JSON fields (and the server
	// allowlist), so one key drives filter, sort and display. Row click and the
	// "+ New" button navigate to the detail / create routes (no inline drawer).
	const columns: Column<Invoice>[] = [
		{ key: 'number', label: 'Number', sortable: true, filter: 'text' },
		{ key: 'participantName', label: 'Participant', sortable: true, filter: 'text' },
		{
			key: 'issueDate',
			label: 'Issued',
			sortable: true,
			filter: 'date',
			cell: (inv) => (inv.issueDate ? inv.issueDate.slice(0, 10) : '—')
		},
		{
			key: 'total',
			label: 'Total',
			sortable: true,
			filter: 'number',
			cell: (inv) => money(inv.total)
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			filter: 'enum',
			values: STATUSES
		},
		{
			key: 'payment',
			label: 'Payment',
			cell: (inv) => paymentLabel(inv.total, inv.totalPaid)
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Invoices</h1>
			<p class="text-sm text-gray-500">NDIS-compliant invoices with price-cap validation.</p>
		</div>
	</section>

	<section>
		{#if invoices.error}
			<p class="mb-3 text-sm text-red-600">{invoices.error}</p>
		{/if}

		<DataTable
			title="Invoices"
			{columns}
			store={invoices}
			rowHref={(inv) => `/invoices/${inv.id}`}
			newHref="/invoices/new"
		/>
	</section>
</div>
