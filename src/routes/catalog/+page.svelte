<script lang="ts">
	import { base } from '$app/paths';
	import { getCatalogItems, getCatalogCategories, bulkDeleteCatalogItems, getCatalogItemsWithTierRate } from '$lib/db/queries/catalog';
	import { getRateTiers } from '$lib/db/queries/rate-tiers';
	import { formatCurrency } from '$lib/utils/format';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import BulkActionBar from '$lib/components/shared/BulkActionBar.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import ImportWizardModal from '$lib/components/import/ImportWizardModal.svelte';
	import { exportCatalog } from '$lib/csv/export-catalog.js';
	import type { CatalogItem } from '$lib/types';

	let search = $state('');
	let selectedCategory = $state('');
	let selectedTierId = $state('');
	let showImportWizard = $state(false);
	let refreshTrigger = $state(0);

	let selectedIds: Set<number> = $state(new Set());
	let showDeleteConfirm = $state(false);

	let tiers = $derived.by(() => {
		refreshTrigger;
		return getRateTiers();
	});

	let categories = $derived.by(() => {
		refreshTrigger;
		return getCatalogCategories();
	});

	let items = $derived.by((): (CatalogItem & { tier_rate?: number })[] => {
		refreshTrigger;
		const tierId = selectedTierId ? Number(selectedTierId) : undefined;
		if (tierId) {
			return getCatalogItemsWithTierRate(search || undefined, selectedCategory || undefined, tierId);
		}
		return getCatalogItems(search || undefined, selectedCategory || undefined);
	});

	$effect(() => {
		// Clear selection when filters change
		search;
		selectedCategory;
		selectedTierId;
		selectedIds = new Set();
	});

	let allSelected = $derived(items.length > 0 && selectedIds.size === items.length);

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
		await bulkDeleteCatalogItems([...selectedIds]);
		selectedIds = new Set();
		showDeleteConfirm = false;
		refreshTrigger++;
	}

	function handleImport() {
		showImportWizard = true;
	}

	function handleImportComplete() {
		showImportWizard = false;
		refreshTrigger++;
	}

	function getDisplayRate(item: CatalogItem & { tier_rate?: number }): number {
		if (selectedTierId && item.tier_rate != null) {
			return item.tier_rate;
		}
		return item.rate;
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Catalog</h1>
		<div class="flex items-center gap-3">
			<Button variant="secondary" size="sm" onclick={exportCatalog}>Export CSV</Button>
			<Button variant="secondary" size="sm" onclick={handleImport}>Import</Button>
			<a href="{base}/catalog/new">
				<Button>New Item</Button>
			</a>
		</div>
	</div>

	<!-- Search & Filter -->
	<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
		<div class="max-w-sm flex-1">
			<SearchInput bind:value={search} placeholder="Search catalog..." />
		</div>
		<select
			bind:value={selectedCategory}
			class="rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		>
			<option value="">All Categories</option>
			{#each categories as cat}
				<option value={cat}>{cat}</option>
			{/each}
		</select>
		{#if tiers.length > 0}
			<select
				bind:value={selectedTierId}
				class="rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			>
				<option value="">Default Rates</option>
				{#each tiers as tier}
					<option value={String(tier.id)}>{tier.name}</option>
				{/each}
			</select>
		{/if}
	</div>

	<!-- Bulk action bar -->
	<BulkActionBar count={selectedIds.size} ondeselect={() => (selectedIds = new Set())}>
		<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>Delete</Button>
	</BulkActionBar>

	<!-- Item list -->
	{#if items.length === 0}
		{#if search || selectedCategory}
			<EmptyState title="No results" message="No catalog items match your search. Try a different term or category." />
		{:else}
			<EmptyState title="No catalog items yet" message="Add your first catalog item to get started.">
				<a href="{base}/catalog/new">
					<Button>New Item</Button>
				</a>
			</EmptyState>
		{/if}
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white">
			<table class="min-w-full divide-y divide-gray-200">
				<thead class="bg-gray-50">
					<tr>
						<th class="w-10 px-4 py-3">
							<input
								type="checkbox"
								checked={allSelected}
								onchange={toggleAll}
								class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
							/>
						</th>
						<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Name</th>
						<th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">Rate</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 sm:table-cell">Unit</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 md:table-cell">Category</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 lg:table-cell">SKU</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200">
					{#each items as item}
						<tr class="transition-colors {selectedIds.has(item.id) ? 'bg-primary-50' : 'hover:bg-gray-50'}">
							<td class="w-10 px-4 py-3">
								<input
									type="checkbox"
									checked={selectedIds.has(item.id)}
									onchange={() => toggleOne(item.id)}
									class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
							</td>
							<td class="px-6 py-4">
								<a href="{base}/catalog/{item.id}" class="font-medium text-primary-600 hover:text-primary-700">
									{item.name}
								</a>
							</td>
							<td class="px-6 py-4 text-right text-sm text-gray-900">
								{formatCurrency(getDisplayRate(item))}
								{#if selectedTierId && item.tier_rate == null}
									<span class="text-xs text-gray-400">(default)</span>
								{/if}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 sm:table-cell">
								{item.unit || '-'}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 md:table-cell">
								{item.category || '-'}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 lg:table-cell">
								{item.sku || '-'}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title="Delete Catalog Items">
	<p class="text-sm text-gray-600">
		Are you sure you want to delete {selectedIds.size} catalog item{selectedIds.size === 1 ? '' : 's'}? This action cannot be undone.
	</p>
	<div class="mt-4 flex justify-end gap-3">
		<Button variant="secondary" size="sm" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
		<Button variant="danger" size="sm" onclick={handleBulkDelete}>Delete</Button>
	</div>
</Modal>

<ImportWizardModal
	open={showImportWizard}
	onclose={() => (showImportWizard = false)}
	oncomplete={handleImportComplete}
/>
