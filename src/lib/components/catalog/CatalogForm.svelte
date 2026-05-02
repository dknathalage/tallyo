<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import type { CatalogItem, CatalogItemWithRates } from '$lib/types';
	import Button from '$lib/components/shared/Button.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	const {
		initialData,
		onsubmit
	}: {
		initialData?: CatalogItem;
		onsubmit: (data: {
			name: string;
			rate: number;
			unit: string;
			category: string;
			sku: string;
			tierRates?: Record<number, number> | undefined;
			metadata?: string | undefined;
		}) => void;
	} = $props();

	let name = $state(untrack(() => initialData?.name ?? ''));
	let rate = $state(untrack(() => initialData?.rate ?? 0));
	let unit = $state(untrack(() => initialData?.unit ?? ''));
	let category = $state(untrack(() => initialData?.category ?? ''));
	let sku = $state(untrack(() => initialData?.sku ?? ''));
	let metadata = $state(untrack(() => initialData?.metadata ?? '{}'));

	let categories = $state<string[]>([]);
	let tiers = $state<{ id: number; name: string }[]>([]);
	let itemWithRates = $state<CatalogItemWithRates | null>(null);

	onMount(async () => {
		const [catalogRes, tiersRes] = await Promise.all([
			fetch('/api/catalog?limit=10000'),
			fetch('/api/rate-tiers')
		]);
		const catalogBody = await catalogRes.json();
		const catalogItems = (catalogBody.data ?? catalogBody) as CatalogItem[];
		categories = [...new Set(catalogItems.map((i: CatalogItem) => i.category).filter(Boolean))];
		tiers = await tiersRes.json() as { id: number; name: string }[];

		// Load existing tier rates if editing
		if (initialData?.id) {
			const res = await fetch(`/api/catalog/${initialData.id}`);
			const data = await res.json() as { itemWithRates?: CatalogItemWithRates } & CatalogItemWithRates;
			itemWithRates = data.itemWithRates ?? data;
		}
	});

	// Tier rate values as a mutable record
	let tierRates: Record<number, string> = $state({});

	// Initialize tier rates from loaded data
	$effect(() => {
		const rates: Record<number, string> = {};
		for (const tier of tiers) {
			if (itemWithRates?.rates[tier.id] !== undefined) {
				rates[tier.id] = String(itemWithRates.rates[tier.id]);
			} else {
				rates[tier.id] = '';
			}
		}
		tierRates = rates;
	});

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();

		// Build tier rates record (only include non-empty values)
		const parsedTierRates: Record<number, number> = {};
		for (const tier of tiers) {
			const val = tierRates[tier.id];
			if (val !== undefined && val !== '') {
				parsedTierRates[tier.id] = parseFloat(val);
			}
		}

		onsubmit({
			name,
			rate,
			unit,
			category,
			sku,
			tierRates: Object.keys(parsedTierRates).length > 0 ? parsedTierRates : undefined,
			metadata: metadata !== '{}' ? metadata : undefined
		});
	}
</script>

<form onsubmit={handleSubmit} class="space-y-4">
	<fieldset class="space-y-4 border-0 p-0 m-0">
		<legend class="sr-only">{i18n.t('a11y.itemDetails')}</legend>
		<div>
			<label for="name" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('catalog.name')} <span class="text-red-500">*</span></label>
			<input
				id="name"
				type="text"
				bind:value={name}
				required
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder={i18n.t('catalog.itemNamePlaceholder')}
			/>
		</div>

		<div>
			<label for="unit" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('catalog.unit')}</label>
			<input
				id="unit"
				type="text"
				bind:value={unit}
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder={i18n.t('catalog.unitPlaceholder')}
			/>
		</div>

		<div>
			<label for="category" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('catalog.category')}</label>
			<input
				id="category"
				type="text"
				bind:value={category}
				list="category-options"
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder={i18n.t('catalog.categoryPlaceholder')}
			/>
			<datalist id="category-options">
				{#each categories as cat (cat)}
					<option value={cat}></option>
				{/each}
			</datalist>
		</div>

		<div>
			<label for="sku" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('catalog.sku')}</label>
			<input
				id="sku"
				type="text"
				bind:value={sku}
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder={i18n.t('catalog.skuPlaceholder')}
			/>
		</div>
	</fieldset>

	<fieldset class="space-y-4 border-0 p-0 m-0">
		<legend class="sr-only">{i18n.t('a11y.pricingSection')}</legend>
		<div>
			<label for="rate" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('catalog.defaultRate')}</label>
			<input
				id="rate"
				type="number"
				min="0"
				step="any"
				bind:value={rate}
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder="0.00"
			/>
		</div>

		<!-- Tier Rates -->
		{#if tiers.length > 0}
			<fieldset class="rounded-lg border border-gray-200 dark:border-gray-700 p-4 m-0">
				<legend class="text-sm font-medium text-gray-700 dark:text-gray-300 px-1">{i18n.t('catalog.tierRates')}</legend>
				<div class="space-y-3">
					{#each tiers as tier (tier.id)}
						<div class="flex items-center gap-3">
							<label for="tier-rate-{tier.id}" class="w-32 text-sm text-gray-600 dark:text-gray-300 truncate" title={tier.name}>
								{tier.name}
							</label>
							<input
								id="tier-rate-{tier.id}"
								type="number"
								min="0"
								step="any"
								bind:value={tierRates[tier.id]}
								class="block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
								placeholder={i18n.t('catalog.useDefaultRate')}
							/>
						</div>
					{/each}
				</div>
				<p class="mt-2 text-xs text-gray-400 dark:text-gray-500">{i18n.t('catalog.tierRatesLeaveBlank')}</p>
			</fieldset>
		{/if}
	</fieldset>

	<div>
		<label for="metadata" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('catalog.metadataJson')}</label>
		<textarea
			id="metadata"
			bind:value={metadata}
			rows="3"
			class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 font-mono text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder={i18n.t('catalog.metadataJson')}
		></textarea>
	</div>

	<div class="flex justify-end gap-3 pt-2">
		<Button variant="secondary" onclick={() => history.back()}>{i18n.t('common.cancel')}</Button>
		<Button type="submit">{initialData ? i18n.t('common.saveChanges') : i18n.t('catalog.createItem')}</Button>
	</div>
</form>
