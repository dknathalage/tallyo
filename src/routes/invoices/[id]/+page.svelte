<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getInvoice, getInvoiceLineItems, deleteInvoice, updateInvoiceStatus } from '$lib/db/queries/invoices.js';
	import { getClient } from '$lib/db/queries/clients.js';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import { exportInvoicePdf } from '$lib/utils/pdf.js';
	import type { Invoice, LineItem } from '$lib/types/index.js';
	import Button from '$lib/components/shared/Button.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';

	let invoice: Invoice | null = $state(null);
	let lineItems: LineItem[] = $state([]);
	let showDeleteConfirm = $state(false);
	let showStatusMenu = $state(false);

	const allStatuses = ['draft', 'sent', 'paid', 'overdue'] as const;

	$effect(() => {
		const id = Number(page.params.id);
		invoice = getInvoice(id);
		if (invoice) {
			lineItems = getInvoiceLineItems(invoice.id);
		}
	});

	async function handleDelete() {
		if (!invoice) return;
		await deleteInvoice(invoice.id);
		goto(`${base}/invoices`);
	}

	async function handleStatusChange(status: string) {
		if (!invoice) return;
		await updateInvoiceStatus(invoice.id, status);
		invoice = getInvoice(invoice.id);
		showStatusMenu = false;
	}

	function handleExportPdf() {
		if (!invoice) return;
		const client = getClient(invoice.client_id);
		if (!client) return;
		exportInvoicePdf(invoice, lineItems, client);
	}
</script>

{#if !invoice}
	<div class="py-12 text-center">
		<p class="text-gray-500">Invoice not found.</p>
		<a href="{base}/invoices" class="mt-2 inline-block text-sm text-primary-600 hover:text-primary-700">Back to invoices</a>
	</div>
{:else}
	<div class="space-y-6">
		<!-- Header -->
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-3">
				<a href="{base}/invoices" class="text-gray-400 transition-colors hover:text-gray-600" aria-label="Back to invoices">
					<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
					</svg>
				</a>
				<h1 class="text-2xl font-bold text-gray-900">{invoice.invoice_number}</h1>
				<StatusBadge status={invoice.status} />
			</div>

			<div class="flex items-center gap-2">
				<!-- Status dropdown -->
				<div class="relative">
					<Button variant="secondary" size="sm" onclick={() => (showStatusMenu = !showStatusMenu)}>
						Status
					</Button>
					{#if showStatusMenu}
						<div class="absolute right-0 z-10 mt-1 w-36 rounded-lg border border-gray-200 bg-white py-1 shadow-lg">
							{#each allStatuses as s}
								<button
									onclick={() => handleStatusChange(s)}
									class="w-full cursor-pointer px-4 py-2 text-left text-sm transition-colors hover:bg-gray-50 {invoice.status === s ? 'font-medium text-primary-600' : 'text-gray-700'}"
								>
									{s.charAt(0).toUpperCase() + s.slice(1)}
								</button>
							{/each}
						</div>
					{/if}
				</div>

				<Button variant="secondary" size="sm" onclick={handleExportPdf}>
					PDF
				</Button>

				<Button variant="secondary" size="sm" onclick={() => goto(`${base}/invoices/${invoice?.id}/edit`)}>
					Edit
				</Button>

				<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>
					Delete
				</Button>
			</div>
		</div>

		<!-- Invoice details -->
		<div class="rounded-lg border border-gray-200 bg-white p-6">
			<div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
				<div>
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500">Client</h3>
					<p class="mt-1 text-sm text-gray-900">{invoice.client_name ?? 'Unknown'}</p>
				</div>
				<div>
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500">Invoice Number</h3>
					<p class="mt-1 text-sm text-gray-900">{invoice.invoice_number}</p>
				</div>
				<div>
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500">Date</h3>
					<p class="mt-1 text-sm text-gray-900">{formatDate(invoice.date)}</p>
				</div>
				<div>
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500">Due Date</h3>
					<p class="mt-1 text-sm text-gray-900">{formatDate(invoice.due_date)}</p>
				</div>
			</div>

			{#if invoice.notes}
				<div class="mt-6">
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500">Notes</h3>
					<p class="mt-1 whitespace-pre-wrap text-sm text-gray-700">{invoice.notes}</p>
				</div>
			{/if}
		</div>

		<!-- Line items -->
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white">
			<table class="min-w-full divide-y divide-gray-200">
				<thead class="bg-gray-50">
					<tr>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500">Description</th>
						<th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500">Qty</th>
						<th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500">Rate</th>
						<th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500">Amount</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200">
					{#each lineItems as item}
						<tr>
							<td class="px-4 py-3 text-sm text-gray-900">{item.description}</td>
							<td class="px-4 py-3 text-right text-sm text-gray-600">{item.quantity}</td>
							<td class="px-4 py-3 text-right text-sm text-gray-600">{formatCurrency(item.rate)}</td>
							<td class="px-4 py-3 text-right text-sm font-medium text-gray-900">{formatCurrency(item.amount)}</td>
						</tr>
					{/each}
				</tbody>
			</table>

			<!-- Totals -->
			<div class="border-t border-gray-200 bg-gray-50 px-4 py-3">
				<div class="flex justify-end">
					<div class="w-64 space-y-1">
						<div class="flex justify-between text-sm">
							<span class="text-gray-600">Subtotal</span>
							<span class="text-gray-900">{formatCurrency(invoice.subtotal)}</span>
						</div>
						<div class="flex justify-between text-sm">
							<span class="text-gray-600">Tax ({invoice.tax_rate}%)</span>
							<span class="text-gray-900">{formatCurrency(invoice.tax_amount)}</span>
						</div>
						<div class="flex justify-between border-t border-gray-300 pt-1 text-sm font-semibold">
							<span class="text-gray-900">Total</span>
							<span class="text-gray-900">{formatCurrency(invoice.total)}</span>
						</div>
					</div>
				</div>
			</div>
		</div>
	</div>

	<ConfirmDialog
		open={showDeleteConfirm}
		title="Delete Invoice"
		message="Are you sure you want to delete invoice {invoice.invoice_number}? This action cannot be undone."
		confirmLabel="Delete"
		confirmVariant="danger"
		onconfirm={handleDelete}
		oncancel={() => (showDeleteConfirm = false)}
	/>
{/if}
