<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getInvoice, getInvoiceLineItems, updateInvoice } from '$lib/db/queries/invoices.js';
	import InvoiceForm from '$lib/components/invoice/InvoiceForm.svelte';
	import type { Invoice, LineItem } from '$lib/types/index.js';

	let invoice: Invoice | null = $state(null);
	let lineItems: LineItem[] = $state([]);

	$effect(() => {
		const id = Number(page.params.id);
		const inv = getInvoice(id);
		invoice = inv;
		if (inv) {
			lineItems = getInvoiceLineItems(inv.id);
		}
	});

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
		items: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number }>
	) {
		if (!invoice) return;
		await updateInvoice(invoice.id, data, items);
		goto(`${base}/invoices/${invoice.id}`);
	}
</script>

{#if !invoice}
	<div class="py-12 text-center">
		<p class="text-gray-500 dark:text-gray-400">Invoice not found.</p>
		<a href="{base}/invoices" class="mt-2 inline-block text-sm text-primary-600 hover:text-primary-700">Back to invoices</a>
	</div>
{:else}
	<div class="space-y-6">
		<div class="flex items-center gap-3">
			<a href="{base}/invoices/{invoice.id}" class="text-gray-400 transition-colors hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300" aria-label="Back to invoice">
				<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
				</svg>
			</a>
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Edit {invoice.invoice_number}</h1>
		</div>

		<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
			<InvoiceForm initialData={invoice} initialLineItems={lineItems} onsubmit={handleSubmit} />
		</div>
	</div>
{/if}
