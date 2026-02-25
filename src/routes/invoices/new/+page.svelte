<script lang="ts">
	import InvoiceForm from '$lib/components/invoice/InvoiceForm.svelte';
	import { createInvoice } from '$lib/db/queries/invoices.js';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';

	async function handleSubmit(
		data: {
			invoice_number: string;
			client_id: number;
			date: string;
			due_date: string;
			subtotal: number;
			tax_rate: number;
			tax_amount: number;
			total: number;
			notes: string;
			status: string;
			business_snapshot: string;
			client_snapshot: string;
			payer_snapshot: string;
		},
		lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number }>
	) {
		await createInvoice(data, lineItems);
		goto(`${base}/invoices`);
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<a href="{base}/invoices" class="text-gray-400 transition-colors hover:text-gray-600" aria-label="Back to invoices">
			<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
			</svg>
		</a>
		<h1 class="text-2xl font-bold text-gray-900">New Invoice</h1>
	</div>

	<div class="rounded-lg border border-gray-200 bg-white p-6">
		<InvoiceForm onsubmit={handleSubmit} />
	</div>
</div>
