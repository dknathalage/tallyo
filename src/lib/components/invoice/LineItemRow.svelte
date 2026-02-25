<script lang="ts">
	import CatalogAutocomplete from '$lib/components/catalog/CatalogAutocomplete.svelte';
	import CatalogBrowseModal from '$lib/components/catalog/CatalogBrowseModal.svelte';
	import { getEffectiveRate } from '$lib/db/queries/catalog.js';
	import type { CatalogItem } from '$lib/types/index.js';

	let {
		item = $bindable(),
		onremove,
		tierId
	}: {
		item: { description: string; quantity: number; rate: number; amount: number; unit?: string };
		onremove: () => void;
		tierId?: number | null;
	} = $props();

	let browseOpen = $state(false);

	function recalculate() {
		item.amount = Math.round(item.quantity * item.rate * 100) / 100;
	}

	function handleCatalogSelect(catalogItem: CatalogItem) {
		item.description = catalogItem.name;
		item.rate = tierId ? getEffectiveRate(catalogItem.id, tierId) : catalogItem.rate;
		item.unit = catalogItem.unit;
		recalculate();
	}
</script>

<div class="flex items-start gap-3">
	<div class="flex-1">
		<div class="flex gap-1">
			<div class="flex-1">
				<CatalogAutocomplete bind:value={item.description} onselect={handleCatalogSelect} {tierId} />
			</div>
			<button
				type="button"
				onclick={() => (browseOpen = true)}
				class="mt-0.5 cursor-pointer rounded p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600"
				aria-label="Browse catalog"
			>
				<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />
				</svg>
			</button>
		</div>
		{#if item.unit}
			<p class="mt-0.5 text-xs text-gray-400">per {item.unit}</p>
		{/if}
	</div>

	<div class="w-24">
		<input
			type="number"
			bind:value={item.quantity}
			oninput={recalculate}
			min="0"
			step="any"
			placeholder="Qty"
			class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		/>
	</div>

	<div class="w-28">
		<input
			type="number"
			bind:value={item.rate}
			oninput={recalculate}
			min="0"
			step="any"
			placeholder="Rate"
			class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		/>
	</div>

	<div class="flex w-28 items-center justify-end py-2 text-sm font-medium text-gray-900">
		${item.amount.toFixed(2)}
	</div>

	<button
		type="button"
		onclick={onremove}
		class="mt-1.5 cursor-pointer rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600"
		aria-label="Remove line item"
	>
		<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
			<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
		</svg>
	</button>
</div>

<CatalogBrowseModal open={browseOpen} onclose={() => (browseOpen = false)} onselect={handleCatalogSelect} {tierId} />
