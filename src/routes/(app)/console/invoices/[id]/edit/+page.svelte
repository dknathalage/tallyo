<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import InvoiceForm from '$lib/components/invoice/InvoiceForm.svelte';
	import type { PageData } from './$types';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { addToast } from '$lib/stores/toast.svelte.js';

	let { data }: { data: PageData } = $props();

	async function handleSubmit(
		updateData: {
			invoice_number: string;
			client_id: number;
			date: string;
			due_date: string;
			payment_terms: string;
			subtotal: number;
			tax_rate: number;
			tax_rate_id: number | null;
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
		try {
			await fetch(`/api/invoices/${data.invoice.id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ ...updateData, lineItems: items })
			});
			goto(`${base}/console/invoices/${data.invoice.id}`);
		} catch (e: any) {
			addToast({ type: 'error', message: e.message || 'Failed to update invoice' });
		}
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<a href="{base}/console/invoices/{data.invoice.id}" class="text-gray-400 transition-colors hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300" aria-label={i18n.t('a11y.backToInvoice')}>
			<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
			</svg>
		</a>
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('invoice.editInvoice', { number: data.invoice.invoice_number })}</h1>
	</div>

	<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
		<InvoiceForm initialData={data.invoice} initialLineItems={data.lineItems} onsubmit={handleSubmit} />
	</div>
</div>
