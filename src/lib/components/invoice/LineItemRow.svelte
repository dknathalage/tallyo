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
		item: { description: string; quantity: number; rate: number; amount: number; unit?: string; notes?: string };
		onremove: () => void;
		tierId?: number | null;
	} = $props();

	let browseOpen = $state(false);
	let notesOpen = $state(false);

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
				onclick={() => (notesOpen = !notesOpen)}
				class="mt-0.5 cursor-pointer rounded p-1.5 transition-colors {notesOpen || item.notes ? 'text-primary-500 hover:bg-primary-50 dark:hover:bg-primary-900/30 hover:text-primary-600' : 'text-gray-400 dark:text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-700 hover:text-gray-600 dark:hover:text-gray-300'}"
				aria-label="Toggle notes"
			>
				<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" d="m16.862 4.487 1.687-1.688a1.875 1.875 0 1 1 2.652 2.652L10.582 16.07a4.5 4.5 0 0 1-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 0 1 1.13-1.897l8.932-8.931Zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0 1 15.75 21H5.25A2.25 2.25 0 0 1 3 18.75V8.25A2.25 2.25 0 0 1 5.25 6H10" />
				</svg>
			</button>
			<button
				type="button"
				onclick={() => (browseOpen = true)}
				class="mt-0.5 cursor-pointer rounded p-1.5 text-gray-400 dark:text-gray-500 transition-colors hover:bg-gray-100 dark:hover:bg-gray-700 hover:text-gray-600 dark:hover:text-gray-300"
				aria-label="Browse catalog"
			>
				<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />
				</svg>
			</button>
		</div>
		{#if item.unit}
			<p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">per {item.unit}</p>
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
			class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
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
			class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		/>
	</div>

	<div class="flex w-28 items-center justify-end py-2 text-sm font-medium text-gray-900 dark:text-white">
		${item.amount.toFixed(2)}
	</div>

	<button
		type="button"
		onclick={onremove}
		class="mt-1.5 cursor-pointer rounded p-1 text-gray-400 dark:text-gray-500 transition-colors hover:bg-red-50 dark:hover:bg-red-900/30 hover:text-red-600"
		aria-label="Remove line item"
	>
		<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
			<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
		</svg>
	</button>
</div>

{#if notesOpen}
	<div class="ml-0 mt-1">
		<textarea
			bind:value={item.notes}
			rows={2}
			placeholder="Work notes..."
			class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		></textarea>
	</div>
{/if}

<CatalogBrowseModal open={browseOpen} onclose={() => (browseOpen = false)} onselect={handleCatalogSelect} {tierId} />
