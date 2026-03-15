<script lang="ts">
	import { goto } from '$app/navigation';
	import { invalidateAll } from '$app/navigation';
	import { base } from '$app/paths';
	import type { PageData } from './$types';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import { exportEstimatePdf } from '$lib/utils/pdf.js';
	import type { Estimate, EstimateLineItem, AuditLogEntry } from '$lib/types/index.js';
	import { parseSnapshot } from '$lib/utils/snapshot.js';
	import Button from '$lib/components/shared/Button.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { addToast } from '$lib/stores/toast.js';

	let { data }: { data: PageData } = $props();

	let estimate: Estimate | null = $state(data.estimate);
	let lineItems: EstimateLineItem[] = $state(data.lineItems);
	let history: AuditLogEntry[] = $state(data.auditHistory);
	let showDeleteConfirm = $state(false);
	let showStatusMenu = $state(false);
	let converting = $state(false);

	const allStatuses = ['draft', 'sent', 'accepted', 'rejected', 'expired'] as const;

	let businessSnap = $derived.by(() => parseSnapshot(estimate?.business_snapshot ?? '{}'));
	let clientSnap = $derived.by(() => parseSnapshot(estimate?.client_snapshot ?? '{}'));
	let payerSnap = $derived.by(() => parseSnapshot(estimate?.payer_snapshot ?? '{}'));



	async function handleDelete() {
		if (!estimate) return;
		await fetch(`/api/estimates/${estimate.id}`, { method: 'DELETE' });
		goto(`${base}/console/estimates`);
	}

	async function handleDuplicate() {
		if (!estimate) return;
		const res = await fetch(`/api/estimates/${estimate.id}`, {
			method: 'PATCH',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'duplicate' })
		});
		const { newId } = await res.json();
		goto(`${base}/console/estimates/${newId}/edit`);
	}

	async function handleStatusChange(status: string) {
		if (!estimate) return;
		await fetch(`/api/estimates/${estimate.id}`, {
			method: 'PATCH',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'status', status })
		});
		const res = await fetch(`/api/estimates/${estimate.id}`);
		estimate = await res.json();
		showStatusMenu = false;
	}

	async function handleConvertToInvoice() {
		if (!estimate) return;
		converting = true;
		try {
			const res = await fetch(`/api/estimates/${estimate.id}`, {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ action: 'convert' })
			});
			const { invoiceId } = await res.json();
			goto(`${base}/console/invoices/${invoiceId}`);
		} catch (e: any) {
			addToast({ type: 'error', message: e.message || 'Failed to convert estimate to invoice' });
		} finally {
			converting = false;
			
		}
	}

	function handleExportPdf() {
		if (!estimate) return;
		exportEstimatePdf(estimate, lineItems);
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
		if (action === 'convert') return 'bg-purple-100 text-purple-800';
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

</script>

{#if !estimate}
	<div class="py-12 text-center">
		<p class="text-gray-500 dark:text-gray-400">{i18n.t('estimate.notFound')}</p>
		<a href="{base}/console/estimates" class="mt-2 inline-block text-sm text-primary-600 hover:text-primary-700">{i18n.t('estimate.backToEstimates')}</a>
	</div>
{:else}
	<div class="space-y-6">
		<!-- Header -->
		<div class="flex items-start justify-between gap-4">
			<div class="flex items-center gap-3">
				<a href="{base}/console/estimates" class="text-gray-400 transition-colors hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300" aria-label={i18n.t('a11y.backToEstimates')}>
					<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
					</svg>
				</a>
				<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{estimate.estimate_number}</h1>
				<StatusBadge status={estimate.status} />
			</div>

			<div class="flex flex-wrap items-center gap-2">
				<!-- Convert to Invoice button -->
				{#if estimate.status === 'accepted' && estimate.converted_invoice_id === null}
					<Button variant="secondary" size="sm" onclick={handleConvertToInvoice} disabled={converting}>
						{converting ? i18n.t('estimate.converting') : i18n.t('estimate.convertToInvoice')}
					</Button>
				{/if}

				<!-- Link to converted invoice -->
				{#if estimate.converted_invoice_id !== null}
					<a href="{base}/console/invoices/{estimate.converted_invoice_id}" class="inline-flex items-center gap-1 rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-primary-600 hover:bg-gray-50 dark:border-gray-600 dark:bg-gray-700 dark:text-primary-400 dark:hover:bg-gray-600">
						{i18n.t('estimate.viewInvoice')}
					</a>
				{/if}

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
									class="w-full cursor-pointer px-4 py-2 text-left text-sm transition-colors hover:bg-gray-50 dark:hover:bg-gray-700 {estimate.status === s ? 'font-medium text-primary-600' : 'text-gray-700 dark:text-gray-300'}"
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

				<Button variant="secondary" size="sm" onclick={() => goto(`${base}/console/estimates/${estimate?.id}/edit`)}>
					{i18n.t('common.edit')}
				</Button>

				<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>
					{i18n.t('common.delete')}
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
						<p class="text-sm font-medium text-gray-900 dark:text-white">{clientSnap.name || estimate.client_name || i18n.t('common.unknown')}</p>
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
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.estimateNumber')}</h3>
						<p class="mt-1 text-sm text-gray-900 dark:text-white">{estimate.estimate_number}</p>
					</div>
					<div>
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.date')}</h3>
						<p class="mt-1 text-sm text-gray-900 dark:text-white">{formatDate(estimate.date)}</p>
					</div>
					<div>
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.validUntil')}</h3>
						<p class="mt-1 text-sm text-gray-900 dark:text-white">{formatDate(estimate.valid_until)}</p>
					</div>
					<div>
						<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.status')}</h3>
						<p class="mt-1"><StatusBadge status={estimate.status} /></p>
					</div>
				</div>
			</div>

			{#if estimate.notes}
				<div class="mt-4 border-t border-gray-200 dark:border-gray-700 pt-4">
					<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.notes')}</h3>
					<p class="mt-1 whitespace-pre-wrap text-sm text-gray-700 dark:text-gray-300">{estimate.notes}</p>
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
							<td class="px-4 py-3 text-right text-sm text-gray-600 dark:text-gray-300">{formatCurrency(item.rate, estimate.currency_code)}</td>
							<td class="px-4 py-3 text-right text-sm font-medium text-gray-900 dark:text-white">{formatCurrency(item.amount, estimate.currency_code)}</td>
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
							<span class="text-gray-900 dark:text-white">{formatCurrency(estimate.subtotal, estimate.currency_code)}</span>
						</div>
						<div class="flex justify-between text-sm">
							<span class="text-gray-600 dark:text-gray-300">{i18n.t('invoice.tax')} ({estimate.tax_rate}%)</span>
							<span class="text-gray-900 dark:text-white">{formatCurrency(estimate.tax_amount, estimate.currency_code)}</span>
						</div>
						<div class="flex justify-between border-t border-gray-300 pt-1 text-sm font-semibold dark:border-gray-600">
							<span class="text-gray-900 dark:text-white">{i18n.t('invoice.total')}</span>
							<span class="text-gray-900 dark:text-white">{formatCurrency(estimate.total, estimate.currency_code)}</span>
						</div>
					</div>
				</div>
			</div>
		</div>

		<!-- Change History -->
		{#if history.length > 0}
			<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('estimate.changeHistory')}</h2>
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
		title={i18n.t('estimate.deleteConfirmTitle')}
		message={i18n.t('estimate.deleteConfirmMessage', { number: estimate.estimate_number })}
		confirmLabel={i18n.t('common.delete')}
		confirmVariant="danger"
		onconfirm={handleDelete}
		oncancel={() => (showDeleteConfirm = false)}
	/>
{/if}
