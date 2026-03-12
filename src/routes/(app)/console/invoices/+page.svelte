<script lang="ts">
	import { repositories } from '$lib/repositories';
		import type { Invoice } from '$lib/types/index.js';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import Button from '$lib/components/shared/Button.svelte';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import BulkActionBar from '$lib/components/shared/BulkActionBar.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import ImportExportBar from '$lib/components/csv/ImportExportBar.svelte';
	import ImportPreviewModal from '$lib/components/csv/ImportPreviewModal.svelte';
	import { exportInvoices } from '$lib/csv/export-invoices.js';
	import { parseInvoicesCsv, commitInvoiceImport } from '$lib/csv/import-invoices.js';
	import { INVOICE_COLUMNS } from '$lib/csv/columns.js';
	import type { ParsedInvoiceImport } from '$lib/csv/types.js';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let search = $state('');
	let statusFilter = $state('');
	let invoices: Invoice[] = $state([]);
	let dueTemplatesCount = $state(0);
	let showPreview = $state(false);
	let previewData: ParsedInvoiceImport | null = $state(null);
	let importTrigger = $state(0);

	let selectedIds: Set<number> = $state(new Set());
	let showDeleteConfirm = $state(false);

	const statuses = ['', 'draft', 'sent', 'paid', 'overdue'] as const;

	$effect(() => {
		importTrigger;
		repositories.invoices.markOverdueInvoices().then(() => {
			invoices = repositories.invoices.getInvoices(search || undefined, statusFilter || undefined);
		});
		dueTemplatesCount = repositories.recurringTemplates.getDueTemplates().length;
		selectedIds = new Set();
	});

	let allSelected = $derived(invoices.length > 0 && selectedIds.size === invoices.length);

	function toggleAll() {
		if (allSelected) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(invoices.map((i) => i.id));
		}
	}

	function toggleOne(id: number) {
		const next = new Set(selectedIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		selectedIds = next;
	}

	async function handleBulkDelete() {
		await repositories.invoices.bulkDeleteInvoices([...selectedIds]);
		selectedIds = new Set();
		showDeleteConfirm = false;
		importTrigger++;
	}

	async function handleBulkStatus(status: string) {
		await repositories.invoices.bulkUpdateInvoiceStatus([...selectedIds], status);
		selectedIds = new Set();
		importTrigger++;
	}

	async function handleImport(file: File) {
		previewData = await parseInvoicesCsv(file);
		showPreview = true;
	}

	async function handleConfirm() {
		if (previewData) {
			await commitInvoiceImport(previewData.groups, previewData.newClientsToCreate);
			showPreview = false;
			previewData = null;
			importTrigger++;
		}
	}
</script>

<div class="space-y-6">
	<!-- Due recurring templates notification -->
	{#if dueTemplatesCount > 0}
		<div class="flex items-center justify-between rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 dark:border-amber-700 dark:bg-amber-900/20">
			<div class="flex items-center gap-2">
				<svg class="h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
				</svg>
				<span class="text-sm font-medium text-amber-800 dark:text-amber-300">
					{i18n.t('recurring.dueNoticeMessage', { count: dueTemplatesCount, plural: dueTemplatesCount === 1 ? '' : 's', verb: dueTemplatesCount === 1 ? 'is' : 'are' })}
				</span>
			</div>
			<a
				href="{base}/console/recurring"
				class="ml-4 shrink-0 text-sm font-medium text-amber-700 underline hover:text-amber-900 dark:text-amber-400 dark:hover:text-amber-200"
			>
				{i18n.t('recurring.viewDue')}
			</a>
		</div>
	{/if}

	<!-- Header -->
	<div class="flex flex-wrap items-center justify-between gap-4">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('invoice.title')}</h1>
		<div class="flex items-center gap-3">
			<ImportExportBar onexport={exportInvoices} onimport={handleImport} />
			<Button onclick={() => goto(`${base}/console/invoices/new`)}>{i18n.t('invoice.newInvoice')}</Button>
		</div>
	</div>

	<!-- Search and filters -->
	<div class="space-y-3">
		<SearchInput bind:value={search} placeholder={i18n.t('invoice.searchPlaceholder')} />

		<div class="flex flex-wrap gap-2">
			{#each statuses as s}
				<button
					onclick={() => (statusFilter = s)}
					class="cursor-pointer rounded-full px-3 py-1 text-sm font-medium transition-colors {statusFilter === s
						? 'bg-primary-600 text-white'
						: 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600'}"
				>
					{s === '' ? i18n.t('status.all') : i18n.t(`status.${s}`)}
				</button>
			{/each}
		</div>
	</div>

	<!-- Bulk action bar -->
	<BulkActionBar count={selectedIds.size} ondeselect={() => (selectedIds = new Set())}>
		<select
			onchange={(e) => {
				const val = e.currentTarget.value;
				if (val) {
					handleBulkStatus(val);
					e.currentTarget.value = '';
				}
			}}
			class="cursor-pointer rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
		>
			<option value="">{i18n.t('status.changeStatus')}</option>
			<option value="draft">{i18n.t('status.draft')}</option>
			<option value="sent">{i18n.t('status.sent')}</option>
			<option value="paid">{i18n.t('status.paid')}</option>
			<option value="overdue">{i18n.t('status.overdue')}</option>
		</select>
		<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>{i18n.t('common.delete')}</Button>
	</BulkActionBar>

	<!-- Invoice list -->
	{#if invoices.length === 0}
		<EmptyState title={i18n.t('invoice.noInvoicesFound')} message={i18n.t('invoice.noInvoicesMessage')}>
			<Button onclick={() => goto(`${base}/console/invoices/new`)}>{i18n.t('invoice.newInvoice')}</Button>
		</EmptyState>
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<caption class="sr-only">{i18n.t('a11y.invoicesTable')}</caption>
				<thead class="bg-gray-50 dark:bg-gray-900">
					<tr>
						<th scope="col" class="w-10 px-4 py-3">
							<input
								type="checkbox"
								checked={allSelected}
								onchange={toggleAll}
								aria-label={i18n.t('a11y.selectAll')}
								class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
							/>
						</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('dashboard.invoice')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.client')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.date')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.status')}</th>
						<th scope="col" class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.total')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each invoices as invoice}
						<tr
							class="transition-colors {selectedIds.has(invoice.id) ? 'bg-primary-50 dark:bg-primary-900/30' : 'hover:bg-gray-50 dark:hover:bg-gray-700'}"
						>
							<td class="w-10 px-4 py-3">
								<input
									type="checkbox"
									checked={selectedIds.has(invoice.id)}
									onchange={() => toggleOne(invoice.id)}
									onclick={(e) => e.stopPropagation()}
									aria-label={i18n.t('a11y.selectInvoice', { number: invoice.invoice_number })}
									class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm font-medium text-primary-600"
								onclick={() => goto(`${base}/console/invoices/${invoice.id}`)}
							>
								{invoice.invoice_number}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm text-gray-900 dark:text-white"
								onclick={() => goto(`${base}/console/invoices/${invoice.id}`)}
							>
								{invoice.client_name ?? i18n.t('common.unknown')}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
								onclick={() => goto(`${base}/console/invoices/${invoice.id}`)}
							>
								{formatDate(invoice.date)}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3"
								onclick={() => goto(`${base}/console/invoices/${invoice.id}`)}
							>
								<StatusBadge status={invoice.status} />
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-right text-sm font-medium text-gray-900"
								onclick={() => goto(`${base}/console/invoices/${invoice.id}`)}
							>
								{formatCurrency(invoice.total, invoice.currency_code)}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title={i18n.t('invoice.bulkDeleteTitle')}>
	<p class="text-sm text-gray-600 dark:text-gray-300">
		{i18n.t('invoice.bulkDeleteMessage', { count: selectedIds.size, plural: selectedIds.size === 1 ? '' : 's' })}
	</p>
	<div class="mt-4 flex justify-end gap-3">
		<Button variant="secondary" size="sm" onclick={() => (showDeleteConfirm = false)}>{i18n.t('common.cancel')}</Button>
		<Button variant="danger" size="sm" onclick={handleBulkDelete}>{i18n.t('common.delete')}</Button>
	</div>
</Modal>

{#if previewData}
	<ImportPreviewModal
		open={showPreview}
		onclose={() => { showPreview = false; }}
		onconfirm={handleConfirm}
		title={i18n.t('invoice.importTitle')}
		totalRows={previewData.totalRows}
		validRows={previewData.validRows.length}
		skippedDuplicates={previewData.skippedDuplicates}
		errors={previewData.errors}
		columns={[...INVOICE_COLUMNS]}
		previewRows={previewData.validRows}
		newClients={previewData.newClientsToCreate}
	/>
{/if}
