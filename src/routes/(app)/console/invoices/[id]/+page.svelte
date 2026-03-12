<script lang="ts">
	import { repositories } from '$lib/repositories';
		import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import { exportInvoicePdf } from '$lib/utils/pdf.js';
	import type { Invoice, LineItem, AuditLogEntry } from '$lib/types/index.js';
	import Button from '$lib/components/shared/Button.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let invoice: Invoice | null = $state(null);
	let lineItems: LineItem[] = $state([]);
	let history: AuditLogEntry[] = $state([]);
	let showDeleteConfirm = $state(false);
	let showStatusMenu = $state(false);

	const allStatuses = ['draft', 'sent', 'paid', 'overdue'] as const;

	let businessSnap = $derived.by(() => parseSnapshot((invoice as any)?.business_snapshot ?? '{}'));
	let clientSnap = $derived.by(() => parseSnapshot((invoice as any)?.client_snapshot ?? '{}'));
	let payerSnap = $derived.by(() => parseSnapshot((invoice as any)?.payer_snapshot ?? '{}'));

	$effect(() => {
		const id = Number(page.params.id);
		const inv = repositories.invoices.getInvoice(id);
		invoice = inv;
		if (inv) {
			lineItems = repositories.invoices.getInvoiceLineItems(inv.id);
			history = repositories.audit.getEntityHistory('invoice', inv.id);
		}
	});

	async function handleDelete() {
		if (!invoice) return;
		await repositories.invoices.deleteInvoice(invoice.id);
		goto(`${base}/console/invoices`);
	}

	async function handleDuplicate() {
		if (!invoice) return;
		const newId = await repositories.invoices.duplicateInvoice(invoice.id);
		goto(`${base}/console/invoices/${newId}/edit`);
	}

	async function handleStatusChange(status: string) {
		if (!invoice) return;
		await repositories.invoices.updateInvoiceStatus(invoice.id, status);
		invoice = repositories.invoices.getInvoice(invoice.id);
		showStatusMenu = false;
	}

	function handleExportPdf() {
		if (!invoice) return;
		exportInvoicePdf(invoice, lineItems);
	}

	function formatTimestamp(ts: string): string {
		const d = new Date(ts + 'Z');
		return d.toLocaleString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	function formatAction(action: string): string {
		return action.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function actionColor(action: string): string {
		if (action === 'create') return 'bg-green-100 text-green-800';
		if (action === 'update') return 'bg-blue-100 text-blue-800';
		if (action === 'delete') return 'bg-red-100 text-red-800';
		if (action === 'status_change') return 'bg-yellow-100 text-yellow-800';
		return 'bg-gray-100 text-gray-800';
	}

	function parseChanges(changesStr: string): Record<string, { old: unknown; new: unknown }> | null {
		try {
			const parsed = JSON.parse(changesStr);
			if (parsed && typeof parsed === 'object' && Object.keys(parsed).length > 0) {
				return parsed;
			}
			return null;
		} catch {
			return null;
		}
	}

	function formatChangeValue(val: unknown): string {
		if (val === null || val === undefined) return '(empty)';
		if (typeof val === 'number') return String(val);
		return String(val) || '(empty)';
	}

	function parseSnapshot(json: string): { name: string; email: string; phone: string; address: string; logo?: string; metadata: Record<string, string> } {
		try {
			const p = JSON.parse(json || '{}');
			return { name: p.name || '', email: p.email || '', phone: p.phone || '', address: p.address || '', logo: p.logo, metadata: p.metadata || {} };
		} catch {
			return { name: '', email: '', phone: '', address: '', metadata: {} };
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
		<!-- Header -->
		<div class="flex items-start justify-between gap-4">
			<div class="flex items-center gap-3">
				<a href="{base}/console/invoices" class="text-gray-400 transition-colors hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300" aria-label={i18n.t('a11y.backToInvoices')}>
					<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
					</svg>
				</a>
				<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{invoice.invoice_number}</h1>
				<StatusBadge status={invoice.status} />
			</div>

			<div class="flex flex-wrap items-center gap-2">
				<!-- Status dropdown -->
				<div class="relative">
					<Button variant="secondary" size="sm" onclick={() => (showStatusMenu = !showStatusMenu)}>
						{i18n.t('invoice.status')}
					</Button>
					{#if showStatusMenu}
						<div class="absolute right-0 z-10 mt-1 w-36 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800">
							{#each allStatuses as s}
								<button
									onclick={() => handleStatusChange(s)}
									class="w-full cursor-pointer px-4 py-2 text-left text-sm transition-colors hover:bg-gray-50 dark:hover:bg-gray-700 {invoice.status === s ? 'font-medium text-primary-600' : 'text-gray-700 dark:text-gray-300'}"
								>
									{i18n.t(`status.${s}`)}
								</button>
							{/each}
						</div>
					{/if}
				</div>

				<Button variant="secondary" size="sm" onclick={handleExportPdf}>
					{i18n.t('invoice.pdf')}
				</Button>

				<Button variant="secondary" size="sm" onclick={handleDuplicate}>
					Duplicate
				</Button>

				<Button variant="secondary" size="sm" onclick={() => goto(`${base}/console/invoices/${invoice?.id}/edit`)}>
					{i18n.t('invoice.edit')}
				</Button>

				<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>
					{i18n.t('invoice.delete')}
				</Button>
			</div>
		</div>

		<!-- Party Details -->
		<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
			{#if businessSnap.name}
				<div class="mb-4">
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.from')}</h3>
					<div class="mt-1">
						<p class="text-sm font-medium text-gray-900 dark:text-white">{businessSnap.name}</p>
						{#if businessSnap.email}<p class="text-sm text-gray-500 dark:text-gray-400">{businessSnap.email}</p>{/if}
						{#if businessSnap.phone}<p class="text-sm text-gray-500 dark:text-gray-400">{businessSnap.phone}</p>{/if}
						{#if businessSnap.address}<p class="whitespace-pre-line text-sm text-gray-500 dark:text-gray-400">{businessSnap.address}</p>{/if}
						{#if Object.keys(businessSnap.metadata).length > 0}
							<div class="mt-1 space-y-0.5">
								{#each Object.entries(businessSnap.metadata) as [key, value]}
									<p class="text-sm text-gray-500 dark:text-gray-400"><span class="font-medium text-gray-700 dark:text-gray-300">{key}:</span> {value}</p>
								{/each}
							</div>
						{/if}
					</div>
				</div>
				<div class="mb-4 border-t border-gray-200 dark:border-gray-700"></div>
			{/if}

			<div class="grid grid-cols-1 gap-6 {payerSnap.name ? 'sm:grid-cols-2' : ''}">
				<div>
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.serviceFor')}</h3>
					<div class="mt-1">
						<p class="text-sm font-medium text-gray-900 dark:text-white">{clientSnap.name || invoice.client_name || i18n.t('common.unknown')}</p>
						{#if clientSnap.email}<p class="text-sm text-gray-500 dark:text-gray-400">{clientSnap.email}</p>{/if}
						{#if clientSnap.phone}<p class="text-sm text-gray-500 dark:text-gray-400">{clientSnap.phone}</p>{/if}
						{#if clientSnap.address}<p class="whitespace-pre-line text-sm text-gray-500 dark:text-gray-400">{clientSnap.address}</p>{/if}
						{#if Object.keys(clientSnap.metadata).length > 0}
							<div class="mt-1 space-y-0.5">
								{#each Object.entries(clientSnap.metadata) as [key, value]}
									<p class="text-sm text-gray-500 dark:text-gray-400"><span class="font-medium text-gray-700 dark:text-gray-300">{key}:</span> {value}</p>
								{/each}
							</div>
						{/if}
					</div>
				</div>

				{#if payerSnap.name}
					<div>
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.billTo')}</h3>
						<div class="mt-1">
							<p class="text-sm font-medium text-gray-900 dark:text-white">{payerSnap.name}</p>
							{#if payerSnap.email}<p class="text-sm text-gray-500 dark:text-gray-400">{payerSnap.email}</p>{/if}
							{#if payerSnap.phone}<p class="text-sm text-gray-500 dark:text-gray-400">{payerSnap.phone}</p>{/if}
							{#if payerSnap.address}<p class="whitespace-pre-line text-sm text-gray-500 dark:text-gray-400">{payerSnap.address}</p>{/if}
							{#if Object.keys(payerSnap.metadata).length > 0}
								<div class="mt-1 space-y-0.5">
									{#each Object.entries(payerSnap.metadata) as [key, value]}
										<p class="text-sm text-gray-500 dark:text-gray-400"><span class="font-medium text-gray-700 dark:text-gray-300">{key}:</span> {value}</p>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				{/if}
			</div>

			<div class="mt-4 border-t border-gray-200 dark:border-gray-700 pt-4">
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-4">
					<div>
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.invoiceNumber')}</h3>
						<p class="mt-1 text-sm text-gray-900 dark:text-white">{invoice.invoice_number}</p>
					</div>
					<div>
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.date')}</h3>
						<p class="mt-1 text-sm text-gray-900 dark:text-white">{formatDate(invoice.date)}</p>
					</div>
					<div>
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.dueDate')}</h3>
						<p class="mt-1 text-sm text-gray-900 dark:text-white">{formatDate(invoice.due_date)}</p>
						{#if invoice.payment_terms && invoice.payment_terms !== 'custom'}
							<p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">{invoice.payment_terms.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}</p>
						{/if}
					</div>
					<div>
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.status')}</h3>
						<p class="mt-1"><StatusBadge status={invoice.status} /></p>
					</div>
				</div>
			</div>

			{#if invoice.notes}
				<div class="mt-4 border-t border-gray-200 dark:border-gray-700 pt-4">
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.notes')}</h3>
					<p class="mt-1 whitespace-pre-wrap text-sm text-gray-700 dark:text-gray-300">{invoice.notes}</p>
				</div>
			{/if}
		</div>

		<!-- Line items -->
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<caption class="sr-only">{i18n.t('a11y.lineItemsTable')}</caption>
				<thead class="bg-gray-50 dark:bg-gray-900">
					<tr>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.description')}</th>
						<th scope="col" class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.qty')}</th>
						<th scope="col" class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.rate')}</th>
						<th scope="col" class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.amount')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each lineItems as item}
						<tr>
							<td class="px-4 py-3 text-sm text-gray-900 dark:text-white">
								{item.description}
								{#if item.notes}
									<p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{item.notes}</p>
								{/if}
							</td>
							<td class="px-4 py-3 text-right text-sm text-gray-600 dark:text-gray-300">{item.quantity}</td>
							<td class="px-4 py-3 text-right text-sm text-gray-600 dark:text-gray-300">{formatCurrency(item.rate, invoice.currency_code)}</td>
							<td class="px-4 py-3 text-right text-sm font-medium text-gray-900 dark:text-white">{formatCurrency(item.amount, invoice.currency_code)}</td>
						</tr>
					{/each}
				</tbody>
			</table>

			<!-- Totals -->
			<div class="border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 px-4 py-3">
				<div class="flex justify-end">
					<div class="w-64 space-y-1">
						<div class="flex justify-between text-sm">
							<span class="text-gray-600 dark:text-gray-300">{i18n.t('invoice.subtotal')}</span>
							<span class="text-gray-900 dark:text-white">{formatCurrency(invoice.subtotal, invoice.currency_code)}</span>
						</div>
						<div class="flex justify-between text-sm">
							<span class="text-gray-600 dark:text-gray-300">{i18n.t('invoice.tax')} ({invoice.tax_rate}%)</span>
							<span class="text-gray-900 dark:text-white">{formatCurrency(invoice.tax_amount, invoice.currency_code)}</span>
						</div>
						<div class="flex justify-between border-t border-gray-300 pt-1 text-sm font-semibold dark:border-gray-600">
							<span class="text-gray-900 dark:text-white">{i18n.t('invoice.total')}</span>
							<span class="text-gray-900 dark:text-white">{formatCurrency(invoice.total, invoice.currency_code)}</span>
						</div>
					</div>
				</div>
			</div>
		</div>

		<!-- Change History -->
		{#if history.length > 0}
			<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('invoice.changeHistory')}</h2>
				<div class="space-y-4">
					{#each history as entry}
						{@const changes = parseChanges(entry.changes)}
						<div class="flex gap-3 border-l-2 border-gray-200 pl-4 dark:border-gray-700">
							<div class="flex-1">
								<div class="flex items-center gap-2">
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium {actionColor(entry.action)}">
										{formatAction(entry.action)}
									</span>
									<span class="text-xs text-gray-500 dark:text-gray-400">{formatTimestamp(entry.created_at)}</span>
								</div>
								{#if changes}
									<div class="mt-1 space-y-0.5">
										{#each Object.entries(changes) as [field, diff]}
											<p class="text-sm text-gray-600 dark:text-gray-300">
												<span class="font-medium">{field}:</span>
												<span class="text-red-600 line-through">{formatChangeValue(diff.old)}</span>
												<span class="text-gray-400 dark:text-gray-500">-></span>
												<span class="text-green-700">{formatChangeValue(diff.new)}</span>
											</p>
										{/each}
									</div>
								{:else if entry.context}
									<p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{entry.context}</p>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>

	<ConfirmDialog
		open={showDeleteConfirm}
		title={i18n.t('invoice.deleteConfirmTitle')}
		message={i18n.t('invoice.deleteConfirmMessage', { number: invoice.invoice_number })}
		confirmLabel={i18n.t('common.delete')}
		confirmVariant="danger"
		onconfirm={handleDelete}
		oncancel={() => (showDeleteConfirm = false)}
	/>
{/if}
