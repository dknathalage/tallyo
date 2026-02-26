<script lang="ts">
	import type { Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import Button from './Button.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let {
		count,
		ondeselect,
		children
	}: {
		count: number;
		ondeselect: () => void;
		children: Snippet;
	} = $props();
</script>

{#if count > 0}
	<div
		class="flex items-center gap-3 rounded-lg border border-primary-200 bg-primary-50 px-4 py-2 dark:border-primary-800 dark:bg-primary-900/30"
		transition:fly={{ y: -8, duration: 150 }}
	>
		<span class="text-sm font-medium text-primary-700 dark:text-primary-300">
			{i18n.t('common.selected', { count })}
		</span>
		<button
			onclick={ondeselect}
			class="cursor-pointer text-sm text-primary-600 underline hover:text-primary-800 dark:text-primary-400 dark:hover:text-primary-200"
		>
			{i18n.t('common.deselect')}
		</button>
		<div class="ml-auto flex items-center gap-2">
			{@render children()}
		</div>
	</div>
{/if}
