<script lang="ts">
	import type { CatalogItem, CatalogItemWithRates } from '$lib/types';
	import { getCatalogCategories, getCatalogItemWithRates } from '$lib/db/queries/catalog';
	import { getRateTiers } from '$lib/db/queries/rate-tiers';
	import Button from '$lib/components/shared/Button.svelte';

	let {
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
			tierRates?: Record<number, number>;
			metadata?: string;
		}) => void;
	} = $props();

	let name = $state(initialData?.name ?? '');
	let rate = $state(initialData?.rate ?? 0);
	let unit = $state(initialData?.unit ?? '');
	let category = $state(initialData?.category ?? '');
	let sku = $state(initialData?.sku ?? '');
	let metadata = $state(initialData?.metadata ?? '{}');

	let categories = $derived(getCatalogCategories());
	let tiers = $derived(getRateTiers());

	// Load existing tier rates if editing
	let itemWithRates: CatalogItemWithRates | null = $derived.by(() => {
		if (initialData?.id) {
			return getCatalogItemWithRates(initialData.id);
		}
		return null;
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
	<div>
		<label for="name" class="block text-sm font-medium text-gray-700">Name <span class="text-red-500">*</span></label>
		<input
			id="name"
			type="text"
			bind:value={name}
			required
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="Item name"
		/>
	</div>

	<div>
		<label for="rate" class="block text-sm font-medium text-gray-700">Default Rate</label>
		<input
			id="rate"
			type="number"
			min="0"
			step="any"
			bind:value={rate}
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="0.00"
		/>
	</div>

	<!-- Tier Rates -->
	{#if tiers.length > 0}
		<div class="rounded-lg border border-gray-200 p-4">
			<h3 class="mb-3 text-sm font-medium text-gray-700">Tier Rates</h3>
			<div class="space-y-3">
				{#each tiers as tier}
					<div class="flex items-center gap-3">
						<label for="tier-rate-{tier.id}" class="w-32 text-sm text-gray-600 truncate" title={tier.name}>
							{tier.name}
						</label>
						<input
							id="tier-rate-{tier.id}"
							type="number"
							min="0"
							step="any"
							bind:value={tierRates[tier.id]}
							class="block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
							placeholder="Use default rate"
						/>
					</div>
				{/each}
			</div>
			<p class="mt-2 text-xs text-gray-400">Leave blank to use the default rate for that tier.</p>
		</div>
	{/if}

	<div>
		<label for="unit" class="block text-sm font-medium text-gray-700">Unit</label>
		<input
			id="unit"
			type="text"
			bind:value={unit}
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="e.g., hour, each, day"
		/>
	</div>

	<div>
		<label for="category" class="block text-sm font-medium text-gray-700">Category</label>
		<input
			id="category"
			type="text"
			bind:value={category}
			list="category-options"
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="e.g., Services, Materials"
		/>
		<datalist id="category-options">
			{#each categories as cat}
				<option value={cat}></option>
			{/each}
		</datalist>
	</div>

	<div>
		<label for="sku" class="block text-sm font-medium text-gray-700">SKU</label>
		<input
			id="sku"
			type="text"
			bind:value={sku}
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="Optional SKU code"
		/>
	</div>

	<div>
		<label for="metadata" class="block text-sm font-medium text-gray-700">Metadata (JSON)</label>
		<textarea
			id="metadata"
			bind:value={metadata}
			rows="3"
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 font-mono text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="JSON metadata"
		></textarea>
	</div>

	<div class="flex justify-end gap-3 pt-2">
		<Button variant="secondary" onclick={() => history.back()}>Cancel</Button>
		<Button type="submit">{initialData ? 'Save Changes' : 'Create Item'}</Button>
	</div>
</form>
