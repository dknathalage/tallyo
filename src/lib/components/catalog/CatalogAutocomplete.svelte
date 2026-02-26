<script lang="ts">
	import { searchCatalogItems, getEffectiveRate } from '$lib/db/queries/catalog.js';
	import type { CatalogItem } from '$lib/types/index.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { announcer } from '$lib/stores/announcer.svelte.js';

	let {
		value = $bindable(),
		onselect,
		tierId
	}: {
		value: string;
		onselect: (item: CatalogItem) => void;
		tierId?: number | null;
	} = $props();

	let suggestions = $state<CatalogItem[]>([]);
	let showDropdown = $state(false);
	let highlightedIndex = $state(-1);
	let blurTimeout: ReturnType<typeof setTimeout> | undefined;

	const listboxId = 'catalog-autocomplete-listbox';

	let activeDescendantId = $derived(
		highlightedIndex >= 0 ? `catalog-option-${highlightedIndex}` : undefined
	);

	function handleInput() {
		if (value.trim().length > 0) {
			suggestions = searchCatalogItems(value);
			showDropdown = suggestions.length > 0;
			highlightedIndex = -1;
			if (suggestions.length > 0) {
				announcer.announce(
					i18n.t('a11y.nSuggestionsAvailable', { count: String(suggestions.length) }),
					'polite'
				);
			}
		} else {
			suggestions = [];
			showDropdown = false;
		}
	}

	function selectItem(item: CatalogItem) {
		value = item.name;
		showDropdown = false;
		suggestions = [];
		highlightedIndex = -1;
		announcer.announce(i18n.t('a11y.suggestionSelected', { name: item.name }), 'polite');
		onselect(item);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!showDropdown) return;

		if (e.key === 'ArrowDown') {
			e.preventDefault();
			highlightedIndex = (highlightedIndex + 1) % suggestions.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			highlightedIndex = highlightedIndex <= 0 ? suggestions.length - 1 : highlightedIndex - 1;
		} else if (e.key === 'Enter' && highlightedIndex >= 0) {
			e.preventDefault();
			selectItem(suggestions[highlightedIndex]);
		} else if (e.key === 'Escape') {
			showDropdown = false;
			highlightedIndex = -1;
		}
	}

	function handleBlur() {
		blurTimeout = setTimeout(() => {
			showDropdown = false;
			highlightedIndex = -1;
		}, 200);
	}

	function handleFocus() {
		if (blurTimeout) {
			clearTimeout(blurTimeout);
		}
		if (value.trim().length > 0) {
			suggestions = searchCatalogItems(value);
			showDropdown = suggestions.length > 0;
		}
	}
</script>

<div class="relative">
	<input
		type="text"
		bind:value
		oninput={handleInput}
		onkeydown={handleKeydown}
		onblur={handleBlur}
		onfocus={handleFocus}
		placeholder={i18n.t('invoice.description')}
		class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		autocomplete="off"
		role="combobox"
		aria-expanded={showDropdown}
		aria-controls={listboxId}
		aria-activedescendant={activeDescendantId}
		aria-autocomplete="list"
		aria-label={i18n.t('a11y.catalogSuggestions')}
	/>

	{#if showDropdown}
		<div
			id={listboxId}
			role="listbox"
			aria-label={i18n.t('a11y.catalogSuggestions')}
			class="absolute left-0 top-full z-20 mt-1 w-full rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 py-1 shadow-lg"
		>
			{#each suggestions as item, i}
				<button
					type="button"
					id="catalog-option-{i}"
					role="option"
					aria-selected={i === highlightedIndex}
					class="w-full cursor-pointer px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-gray-700 {i === highlightedIndex ? 'bg-primary-50 dark:bg-primary-900/50' : ''}"
					onmousedown={() => selectItem(item)}
				>
					<span class="font-medium text-gray-900 dark:text-white">{item.name}</span>
					<span class="ml-2 text-gray-500 dark:text-gray-400">
						${tierId ? getEffectiveRate(item.id, tierId).toFixed(2) : item.rate.toFixed(2)}
					</span>
					{#if item.category}
						<span class="ml-2 text-xs text-gray-400 dark:text-gray-500">{item.category}</span>
					{/if}
				</button>
			{/each}
		</div>
	{/if}
</div>
