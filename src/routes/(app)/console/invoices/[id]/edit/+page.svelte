<script lang="ts">
	import { repositories } from '$lib/repositories';
		import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import InvoiceForm from '$lib/components/invoice/InvoiceForm.svelte';
	import type { Invoice, LineItem } from '$lib/types/index.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { addToast } from '$lib/stores/toast.js';

	let invoice: Invoice | null = $state(null);
	let lineItems: LineItem[] = $state([]);

	$effect(() => {
		const id = Number(page.params.id);
		const inv = repositories.invoices.getInvoice(id);
		invoice = inv;
		if (inv) {
			lineItems = repositories.invoices.getInvoiceLineItems(inv.id);
		}
	});

	async function handleSubmit(
		data: {
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
		if (!invoice) return;
		try {
			await repositories.invoices.updateInvoice(invoice.id, data, items);
			goto(`${base}/console/invoices/${invoice.id}`);
		} catch (e: any) {
			addToast({ type: 'error', message: e.message || 'Failed to update invoice' });
		}
	}
</script>

{#if !invoice}
	<div class="py-12 text-center">
		<p class="text-gray-500 dark:text-gray-400">{i18n.t('invoice.notFound')}</p>
		<a href="{base}/console/invoices" class="mt-2 inline-block text-sm text-primary-600 hover:text-primary-700">{i18n.t('invoice.backToInvoices')}</a>
	</div>
{:else}
	<div class="space-y-6">
		<div class="flex items-center gap-3">
			<a href="{base}/console/invoices/{invoice.id}" class="text-gray-400 transition-colors hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300" aria-label={i18n.t('a11y.backToInvoice')}>
				<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
				</svg>
			</a>
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('invoice.editInvoice', { number: invoice.invoice_number })}</h1>
		</div>

		<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
			<InvoiceForm initialData={invoice} initialLineItems={lineItems} onsubmit={handleSubmit} />
		</div>
	</div>
{/if}
