<script lang="ts">
	import Modal from '$lib/components/shared/Modal.svelte';
	import type { CatalogItem } from '$lib/types/index.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	// eslint-disable-next-line svelte/no-unused-props -- `tierId` is reserved for future tier-aware pricing API
	const {
		open = false,
		onclose,
		onselect
	}: {
		open: boolean;
		onclose: () => void;
		onselect: (item: CatalogItem) => void;
		tierId?: number | null | undefined;
	} = $props();

	let search = $state('');
	let selectedCategory = $state('');
	let allItems = $state<CatalogItem[]>([]);

	$effect(() => {
		if (open) {
			void fetch('/api/catalog').then(r => r.json() as Promise<CatalogItem[]>).then(d => { allItems = d; });
			// Get categories from loaded items
		}
	});

	const categories = $derived(open ? [...new Set(allItems.map((i: CatalogItem) => i.category).filter(Boolean))] : []);
	const items = $derived(
		open ? allItems.filter((item: CatalogItem) => {
			const matchesSearch = !search || item.name.toLowerCase().includes(search.toLowerCase());
			const matchesCategory = !selectedCategory || item.category === selectedCategory;
			return matchesSearch && matchesCategory;
		}) : []
	);

	function handleSelect(item: CatalogItem) {
		onselect(item);
		onclose();
	}
</script>

<Modal {open} {onclose} title={i18n.t('catalog.browseTitle')} maxWidth="max-w-2xl">
	<div class="space-y-4">
		<!-- Search and filter -->
		<div class="flex gap-3">
			<div class="flex-1">
				<input
					type="text"
					bind:value={search}
					placeholder={i18n.t('catalog.searchCatalogPlaceholder')}
					class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				/>
			</div>
			<div class="w-40">
				<select
					bind:value={selectedCategory}
					class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				>
					<option value="">{i18n.t('catalog.allCategories')}</option>
					{#each categories as category (category)}
						<option value={category}>{category}</option>
					{/each}
				</select>
			</div>
		</div>

		<!-- Items list -->
		<div class="max-h-96 overflow-y-auto">
			{#if items.length === 0}
				<div class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
					{i18n.t('catalog.noCatalogItems')}
				</div>
			{:else}
				<div class="divide-y divide-gray-100 dark:divide-gray-800">
					{#each items as item (item.id)}
						<button
							type="button"
							class="flex w-full cursor-pointer items-center justify-between px-3 py-3 text-left transition-colors hover:bg-gray-50 dark:hover:bg-gray-700"
							onclick={() => handleSelect(item)}
						>
							<div>
								<div class="text-sm font-medium text-gray-900 dark:text-white">{item.name}</div>
								{#if item.category}
									<div class="text-xs text-gray-400 dark:text-gray-500">{item.category}</div>
								{/if}
							</div>
							<div class="text-right">
								<div class="text-sm font-medium text-gray-900 dark:text-white">
									${item.rate.toFixed(2)}
								</div>
								{#if item.unit}
									<div class="text-xs text-gray-400 dark:text-gray-500">{i18n.t('common.per', { unit: item.unit })}</div>
								{/if}
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</Modal>
