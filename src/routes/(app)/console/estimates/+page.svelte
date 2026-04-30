<script lang="ts">
	import type { Estimate } from '$lib/types/index.js';
	import type { PageData } from './$types';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import Button from '$lib/components/shared/Button.svelte';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import BulkActionBar from '$lib/components/shared/BulkActionBar.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import Pagination from '$lib/components/shared/Pagination.svelte';
	import ImportExportBar from '$lib/components/csv/ImportExportBar.svelte';
	import ImportPreviewModal from '$lib/components/csv/ImportPreviewModal.svelte';
	import { exportEstimates } from '$lib/csv/export-estimates.js';
	import { parseEstimatesCsv } from '$lib/csv/import-estimates.js';
	import { ESTIMATE_COLUMNS } from '$lib/csv/columns.js';
	import type { ParsedEstimateImport } from '$lib/csv/types.js';
	import { goto } from '$app/navigation';
	import { invalidateAll } from '$app/navigation';
	import { base } from '$app/paths';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let { data }: { data: PageData } = $props();

	let search = $state('');
	let statusFilter = $state('');
	let showPreview = $state(false);
	let previewData: ParsedEstimateImport | null = $state(null);

	let selectedIds: Set<number> = $state(new Set());
	let showDeleteConfirm = $state(false);

	const statuses = ['', 'draft', 'sent', 'accepted', 'rejected', 'expired'] as const;
	let paginationResult = $derived(data.estimatesResult);

	let estimates: Estimate[] = $derived(
		paginationResult.data.filter((est: Estimate) => {
			const matchesSearch = !search || est.estimate_number.toLowerCase().includes(search.toLowerCase()) || (est.client_name ?? '').toLowerCase().includes(search.toLowerCase());
			const matchesStatus = !statusFilter || est.status === statusFilter;
			return matchesSearch && matchesStatus;
		})
	);

	let allSelected = $derived(estimates.length > 0 && selectedIds.size === estimates.length);

	function toggleAll() {
		if (allSelected) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(estimates.map((e) => e.id));
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
		await fetch('/api/estimates', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'bulk-delete', ids: [...selectedIds] })
		});
		selectedIds = new Set();
		showDeleteConfirm = false;
		await invalidateAll();
	}

	async function handleBulkStatus(status: string) {
		await fetch('/api/estimates', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'bulk-status', ids: [...selectedIds], status })
		});
		selectedIds = new Set();
		await invalidateAll();
	}

	async function handleImport(file: File) {
		previewData = await parseEstimatesCsv(file);
		showPreview = true;
	}

	async function handleConfirm() {
		if (previewData) {
			for (const group of previewData.groups) {
				const { lineItems, ...estimateData } = group;
				await fetch('/api/estimates', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ ...estimateData, lineItems })
				});
			}
			showPreview = false;
			previewData = null;
			await invalidateAll();
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex flex-wrap items-center justify-between gap-4">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('estimate.title')}</h1>
		<div class="flex items-center gap-3">
			<ImportExportBar onexport={exportEstimates} onimport={handleImport} />
			<Button onclick={() => goto(`${base}/console/estimates/new`)}>{i18n.t('estimate.newEstimate')}</Button>
		</div>
	</div>

	<!-- Search and filters -->
	<div class="space-y-3">
		<SearchInput bind:value={search} placeholder={i18n.t('estimate.searchPlaceholder')} />

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
			<option value="accepted">{i18n.t('status.accepted')}</option>
			<option value="rejected">{i18n.t('status.rejected')}</option>
			<option value="expired">{i18n.t('status.expired')}</option>
		</select>
		<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>{i18n.t('common.delete')}</Button>
	</BulkActionBar>

	<!-- Estimate list -->
	{#if estimates.length === 0}
		<EmptyState title={i18n.t('estimate.noEstimatesFound')} message={i18n.t('estimate.noEstimatesMessage')}>
			<Button onclick={() => goto(`${base}/console/estimates/new`)}>{i18n.t('estimate.newEstimate')}</Button>
		</EmptyState>
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<caption class="sr-only">{i18n.t('a11y.estimatesTable')}</caption>
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
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('dashboard.estimate')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.client')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.date')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.validUntil')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.status')}</th>
						<th scope="col" class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('estimate.total')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each estimates as estimate}
						<tr
							class="transition-colors {selectedIds.has(estimate.id) ? 'bg-primary-50 dark:bg-primary-900/30' : 'hover:bg-gray-50 dark:hover:bg-gray-700'}"
						>
							<td class="w-10 px-4 py-3">
								<input
									type="checkbox"
									checked={selectedIds.has(estimate.id)}
									onchange={() => toggleOne(estimate.id)}
									onclick={(e) => e.stopPropagation()}
									aria-label={i18n.t('a11y.selectEstimate', { number: estimate.estimate_number })}
									class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm font-medium text-primary-600"
								onclick={() => goto(`${base}/console/estimates/${estimate.id}`)}
							>
								{estimate.estimate_number}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm text-gray-900 dark:text-white"
								onclick={() => goto(`${base}/console/estimates/${estimate.id}`)}
							>
								{estimate.client_name ?? i18n.t('common.unknown')}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
								onclick={() => goto(`${base}/console/estimates/${estimate.id}`)}
							>
								{formatDate(estimate.date)}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
								onclick={() => goto(`${base}/console/estimates/${estimate.id}`)}
							>
								{formatDate(estimate.valid_until)}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3"
								onclick={() => goto(`${base}/console/estimates/${estimate.id}`)}
							>
								<StatusBadge status={estimate.status} />
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-right text-sm font-medium text-gray-900"
								onclick={() => goto(`${base}/console/estimates/${estimate.id}`)}
							>
								{formatCurrency(estimate.total, estimate.currency_code)}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<Pagination
			total={paginationResult.total}
			currentPage={paginationResult.page}
			totalPages={paginationResult.totalPages}
			hasNextPage={paginationResult.hasNextPage}
			hasPrevPage={paginationResult.hasPrevPage}
		/>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title={i18n.t('estimate.bulkDeleteTitle')}>
	<p class="text-sm text-gray-600 dark:text-gray-300">
		{i18n.t('estimate.bulkDeleteMessage', { count: selectedIds.size, plural: selectedIds.size === 1 ? '' : 's' })}
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
		title={i18n.t('estimate.importTitle')}
		totalRows={previewData.totalRows}
		validRows={previewData.validRows.length}
		skippedDuplicates={previewData.skippedDuplicates}
		errors={previewData.errors}
		columns={[...ESTIMATE_COLUMNS]}
		previewRows={previewData.validRows}
		newClients={previewData.newClientsToCreate}
	/>
{/if}
