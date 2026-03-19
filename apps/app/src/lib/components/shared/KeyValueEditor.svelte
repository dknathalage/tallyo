<script lang="ts">
	import type { KeyValuePair } from '$lib/types';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let {
		pairs = $bindable(),
		readonly = false,
		addLabel = 'Add Field'
	}: {
		pairs: KeyValuePair[];
		readonly?: boolean;
		addLabel?: string;
	} = $props();

	function addPair() {
		pairs.push({ key: '', value: '' });
	}

	function removePair(index: number) {
		pairs.splice(index, 1);
	}
</script>

<div class="space-y-2">
	{#each pairs as _, i}
		<div class="flex flex-col items-stretch gap-2 sm:flex-row sm:items-center">
			<input
				type="text"
				bind:value={pairs[i].key}
				placeholder={i18n.t('common.fieldName')}
				{readonly}
				class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500 sm:w-1/3 {readonly ? 'bg-gray-50 dark:bg-gray-900' : ''}"
			/>
			<input
				type="text"
				bind:value={pairs[i].value}
				placeholder={i18n.t('common.value')}
				{readonly}
				class="flex-1 rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500 {readonly ? 'bg-gray-50 dark:bg-gray-900' : ''}"
			/>
			{#if !readonly}
				<button
					type="button"
					onclick={() => removePair(i)}
					class="cursor-pointer rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:text-gray-500 dark:hover:bg-red-900/30 dark:hover:text-red-400"
					aria-label={i18n.t('a11y.removeField')}
				>
					<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			{/if}
		</div>
	{/each}
	{#if !readonly}
		<button
			type="button"
			onclick={addPair}
			class="cursor-pointer text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
		>
			+ {addLabel}
		</button>
	{/if}
</div>
