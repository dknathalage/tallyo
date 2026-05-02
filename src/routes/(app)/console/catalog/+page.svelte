<script lang="ts">
	import { resolve } from '$app/paths';
	import type { PageData } from './$types';
	import { formatCurrency } from '$lib/utils/format';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import BulkActionBar from '$lib/components/shared/BulkActionBar.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import Pagination from '$lib/components/shared/Pagination.svelte';
	import ImportWizardModal from '$lib/components/import/ImportWizardModal.svelte';
	import { exportCatalog } from '$lib/csv/export-catalog.js';
	import type { CatalogItem } from '$lib/types';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { invalidateAll } from '$app/navigation';
	import { apiFetch } from '$lib/utils/api.js';

	const { data }: { data: PageData } = $props();

	let search = $state('');
	let selectedCategory = $state('');
	let selectedTierId = $state('');
	let showImportWizard = $state(false);

	let selectedIds: Set<number> = $state(new Set());
	let showDeleteConfirm = $state(false);

	const tiers = $derived(data.rateTiers);
	const categories = $derived(data.categories);
	const paginationResult = $derived(data.catalogResult);

	const items = $derived.by((): (CatalogItem & { tier_rate?: number })[] => {
		return paginationResult.data.filter((item: CatalogItem) => {
			const matchesSearch = !search || item.name.toLowerCase().includes(search.toLowerCase()) || (item.sku || '').toLowerCase().includes(search.toLowerCase());
			const matchesCategory = !selectedCategory || item.category === selectedCategory;
			return matchesSearch && matchesCategory;
		});
	});

	$effect(() => {
		// Clear selection when filters change
		void search;
		void selectedCategory;
		void selectedTierId;
		selectedIds = new Set();
	});

	const allSelected = $derived(items.length > 0 && selectedIds.size === items.length);

	function toggleAll() {
		if (allSelected) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(items.map((i) => i.id));
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
		const ids = [...selectedIds];
		selectedIds = new Set();
		showDeleteConfirm = false;
		const res = await apiFetch('/api/catalog', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'bulk-delete', ids })
		});
		if (res.ok) await invalidateAll();
	}

	function handleImport() {
		showImportWizard = true;
	}

	async function handleImportComplete() {
		showImportWizard = false;
		await invalidateAll();
	}

	function getDisplayRate(item: CatalogItem & { tier_rate?: number }): number {
		if (selectedTierId && item.tier_rate !== undefined) {
			return item.tier_rate;
		}
		return item.rate;
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('catalog.title')}</h1>
		<div class="flex items-center gap-3">
			<Button variant="secondary" size="sm" onclick={exportCatalog}>{i18n.t('csv.exportCsv')}</Button>
			<Button variant="secondary" size="sm" onclick={handleImport}>{i18n.t('csv.import')}</Button>
			<a href={resolve('/(app)/console/catalog/new')}>
				<Button>{i18n.t('catalog.newItem')}</Button>
			</a>
		</div>
	</div>

	<!-- Search & Filter -->
	<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
		<div class="max-w-sm flex-1">
			<SearchInput bind:value={search} placeholder={i18n.t('catalog.searchPlaceholder')} />
		</div>
		<select
			bind:value={selectedCategory}
			class="rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
		>
			<option value="">{i18n.t('catalog.allCategories')}</option>
			{#each categories as cat}
				<option value={cat}>{cat}</option>
			{/each}
		</select>
		{#if tiers.length > 0}
			<select
				bind:value={selectedTierId}
				class="rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
			>
				<option value="">{i18n.t('catalog.defaultRates')}</option>
				{#each tiers as tier}
					<option value={String(tier.id)}>{tier.name}</option>
				{/each}
			</select>
		{/if}
	</div>

	<!-- Bulk action bar -->
	<BulkActionBar count={selectedIds.size} ondeselect={() => (selectedIds = new Set())}>
		<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>{i18n.t('common.delete')}</Button>
	</BulkActionBar>

	<!-- Item list -->
	{#if items.length === 0}
		{#if search || selectedCategory}
			<EmptyState title={i18n.t('common.noResults')} message={i18n.t('catalog.noResultsMessage')} />
		{:else}
			<EmptyState title={i18n.t('catalog.noItems')} message={i18n.t('catalog.noItemsMessage')}>
				<a href={resolve('/(app)/console/catalog/new')}>
					<Button>{i18n.t('catalog.newItem')}</Button>
				</a>
			</EmptyState>
		{/if}
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<thead class="bg-gray-50 dark:bg-gray-900">
					<tr>
						<th class="w-10 px-4 py-3">
							<input
								type="checkbox"
								checked={allSelected}
								onchange={toggleAll}
								class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
							/>
						</th>
						<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('catalog.name')}</th>
						<th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('catalog.rate')}</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 sm:table-cell">{i18n.t('catalog.unit')}</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 md:table-cell">{i18n.t('catalog.category')}</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 lg:table-cell">{i18n.t('catalog.sku')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each items as item}
						<tr class="transition-colors {selectedIds.has(item.id) ? 'bg-primary-50 dark:bg-primary-900/30' : 'hover:bg-gray-50 dark:hover:bg-gray-700'}">
							<td class="w-10 px-4 py-3">
								<input
									type="checkbox"
									checked={selectedIds.has(item.id)}
									onchange={() => toggleOne(item.id)}
									class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
							</td>
							<td class="px-6 py-4">
								<a href={resolve('/(app)/console/catalog/[id]', { id: String(item.id) })} class="font-medium text-primary-600 hover:text-primary-700">
									{item.name}
								</a>
							</td>
							<td class="px-6 py-4 text-right text-sm text-gray-900 dark:text-white">
								{formatCurrency(getDisplayRate(item))}
								{#if selectedTierId && item.tier_rate === undefined}
									<span class="text-xs text-gray-400 dark:text-gray-500">{i18n.t('common.default')}</span>
								{/if}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 sm:table-cell">
								{item.unit || '-'}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 md:table-cell">
								{item.category || '-'}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 lg:table-cell">
								{item.sku || '-'}
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

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title={i18n.t('catalog.bulkDeleteTitle')}>
	<p class="text-sm text-gray-600 dark:text-gray-300">
		{i18n.t('catalog.bulkDeleteMessage', { count: selectedIds.size, plural: selectedIds.size === 1 ? '' : 's' })}
	</p>
	<div class="mt-4 flex justify-end gap-3">
		<Button variant="secondary" size="sm" onclick={() => (showDeleteConfirm = false)}>{i18n.t('common.cancel')}</Button>
		<Button variant="danger" size="sm" onclick={handleBulkDelete}>{i18n.t('common.delete')}</Button>
	</div>
</Modal>

<ImportWizardModal
	open={showImportWizard}
	onclose={() => (showImportWizard = false)}
	oncomplete={handleImportComplete}
/>
