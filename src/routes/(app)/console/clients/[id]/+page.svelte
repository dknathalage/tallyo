<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getClient, updateClient, deleteClient } from '$lib/db/queries/clients';
	import { getClientInvoices } from '$lib/db/queries/invoices';
	import { getClientEstimates } from '$lib/db/queries/estimates';
	import { getEntityHistory } from '$lib/db/queries/audit';
	import { getPayer } from '$lib/db/queries/payers';
	import type { AuditLogEntry } from '$lib/types/index.js';
	import ClientForm from '$lib/components/client/ClientForm.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import { formatCurrency, formatDate } from '$lib/utils/format';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let clientId = $derived(Number(page.params.id));
	let client = $derived(getClient(clientId));
	let invoices = $derived(getClientInvoices(clientId));
	let estimates = $derived(getClientEstimates(clientId));
	let history = $derived(getEntityHistory('client', clientId));
	let payer = $derived(client?.payer_id ? getPayer(client.payer_id) : null);

	let editing = $state(false);
	let showDeleteConfirm = $state(false);

	function parseMetadataObj(metaStr?: string): Record<string, string> {
		try {
			const obj = JSON.parse(metaStr || '{}');
			return typeof obj === 'object' ? obj : {};
		} catch {
			return {};
		}
	}

	async function handleUpdate(data: { name: string; email: string; phone: string; address: string; metadata: string; payer_id: number | null }) {
		await updateClient(clientId, data);
		editing = false;
	}

	async function handleDelete() {
		await deleteClient(clientId);
		goto(`${base}/console/clients`);
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

{#if !client}
	<EmptyState title={i18n.t('client.notFound')} message={i18n.t('client.notFoundMessage')}>
		<a href="{base}/console/clients">
			<Button variant="secondary">{i18n.t('client.backToClients')}</Button>
		</a>
	</EmptyState>
{:else}
	<div class="space-y-6">
		<!-- Header -->
		<div class="flex items-center justify-between">
			<div>
				<a href="{base}/console/clients" class="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200">&larr; {i18n.t('client.backToClients')}</a>
				<h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{client.name}</h1>
			</div>
			<div class="flex gap-2">
				{#if !editing}
					<Button variant="secondary" onclick={() => (editing = true)}>{i18n.t('common.edit')}</Button>
				{/if}
				<Button variant="danger" onclick={() => (showDeleteConfirm = true)}>{i18n.t('common.delete')}</Button>
			</div>
		</div>

		<!-- Client details / Edit form -->
		<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
			{#if editing}
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('client.editClient')}</h2>
				<ClientForm initialData={client} onsubmit={handleUpdate} />
			{:else}
				<dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.email')}</dt>
						<dd class="mt-1 text-sm text-gray-900 dark:text-white">{client.email || '-'}</dd>
					</div>
					<div>
						<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.phone')}</dt>
						<dd class="mt-1 text-sm text-gray-900 dark:text-white">{client.phone || '-'}</dd>
					</div>
					<div class="sm:col-span-2">
						<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.address')}</dt>
						<dd class="mt-1 whitespace-pre-line text-sm text-gray-900 dark:text-white">{client.address || '-'}</dd>
					</div>
					{#if Object.keys(parseMetadataObj(client.metadata)).length > 0}
						<div class="sm:col-span-2">
							<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.additionalFields')}</dt>
							<dd class="mt-1">
								<div class="space-y-1">
									{#each Object.entries(parseMetadataObj(client.metadata)) as [key, value]}
										<div class="text-sm">
											<span class="font-medium text-gray-700 dark:text-gray-300">{key}:</span>
											<span class="text-gray-900 dark:text-white">{value}</span>
										</div>
									{/each}
								</div>
							</dd>
						</div>
					{/if}
				</dl>
				{#if payer}
					<div class="mt-6 border-t border-gray-200 pt-4 dark:border-gray-700">
						<h3 class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.billToPayer')}</h3>
						<div class="mt-2">
							<p class="text-sm font-medium text-gray-900 dark:text-white">{payer.name}</p>
							{#if payer.email}<p class="text-sm text-gray-500 dark:text-gray-400">{payer.email}</p>{/if}
							{#if payer.phone}<p class="text-sm text-gray-500 dark:text-gray-400">{payer.phone}</p>{/if}
							{#if payer.address}<p class="mt-1 whitespace-pre-line text-sm text-gray-500 dark:text-gray-400">{payer.address}</p>{/if}
							{#if Object.keys(parseMetadataObj(payer.metadata)).length > 0}
								<div class="mt-2 space-y-1">
									{#each Object.entries(parseMetadataObj(payer.metadata)) as [key, value]}
										<div class="text-sm">
											<span class="font-medium text-gray-700 dark:text-gray-300">{key}:</span>
											<span class="text-gray-900 dark:text-white">{value}</span>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				{/if}
			{/if}
		</div>

		<!-- Client invoices -->
		<div>
			<div class="flex items-center justify-between">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('client.invoices')}</h2>
				<a href="{base}/console/invoices/new?client_id={clientId}">
					<Button size="sm">{i18n.t('client.newInvoice')}</Button>
				</a>
			</div>

			{#if invoices.length === 0}
				<div class="mt-4">
					<EmptyState title={i18n.t('client.noInvoices')} message={i18n.t('client.noInvoicesMessage')} />
				</div>
			{:else}
				<div class="mt-4 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
					<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
						<thead class="bg-gray-50 dark:bg-gray-900">
							<tr>
								<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('client.invoiceNumber')}</th>
								<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 sm:table-cell">{i18n.t('client.date')}</th>
								<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('client.status')}</th>
								<th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('client.total')}</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
							{#each invoices as invoice}
								<tr class="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700">
									<td class="px-6 py-4">
										<a href="{base}/console/invoices/{invoice.id}" class="font-medium text-primary-600 hover:text-primary-700">
											{invoice.invoice_number}
										</a>
									</td>
									<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 sm:table-cell">
										{formatDate(invoice.date)}
									</td>
									<td class="px-6 py-4">
										<StatusBadge status={invoice.status} />
									</td>
									<td class="px-6 py-4 text-right text-sm font-medium text-gray-900 dark:text-white">
										{formatCurrency(invoice.total, invoice.currency_code)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>

		<!-- Client estimates -->
		<div>
			<div class="flex items-center justify-between">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('client.estimates')}</h2>
				<a href="{base}/estimates/new">
					<Button size="sm">{i18n.t('client.newEstimate')}</Button>
				</a>
			</div>

			{#if estimates.length === 0}
				<div class="mt-4">
					<EmptyState title={i18n.t('client.noEstimates')} message={i18n.t('client.noEstimatesMessage')} />
				</div>
			{:else}
				<div class="mt-4 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
					<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
						<thead class="bg-gray-50 dark:bg-gray-900">
							<tr>
								<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('client.estimateNumber')}</th>
								<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 sm:table-cell">{i18n.t('client.date')}</th>
								<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('client.status')}</th>
								<th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('client.total')}</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
							{#each estimates as estimate}
								<tr class="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700">
									<td class="px-6 py-4">
										<a href="{base}/estimates/{estimate.id}" class="font-medium text-primary-600 hover:text-primary-700">
											{estimate.estimate_number}
										</a>
									</td>
									<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 sm:table-cell">
										{formatDate(estimate.date)}
									</td>
									<td class="px-6 py-4">
										<StatusBadge status={estimate.status} />
									</td>
									<td class="px-6 py-4 text-right text-sm font-medium text-gray-900 dark:text-white">
										{formatCurrency(estimate.total, estimate.currency_code)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>

		<!-- Change History -->
		{#if history.length > 0}
			<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('client.changeHistory')}</h2>
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
									<p class="mt-1 text-sm text-gray-600">{entry.context}</p>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>

	<!-- Delete confirmation -->
	<ConfirmDialog
		open={showDeleteConfirm}
		title={i18n.t('client.deleteConfirmTitle')}
		message={i18n.t('client.deleteConfirmMessage', { name: client.name })}
		confirmLabel={i18n.t('common.delete')}
		confirmVariant="danger"
		onconfirm={handleDelete}
		oncancel={() => (showDeleteConfirm = false)}
	/>
{/if}
