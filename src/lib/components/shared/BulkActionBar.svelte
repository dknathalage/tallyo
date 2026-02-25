<script lang="ts">
	import type { Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import Button from './Button.svelte';

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
		class="flex items-center gap-3 rounded-lg border border-primary-200 bg-primary-50 px-4 py-2"
		transition:fly={{ y: -8, duration: 150 }}
	>
		<span class="text-sm font-medium text-primary-700">
			{count} selected
		</span>
		<button
			onclick={ondeselect}
			class="cursor-pointer text-sm text-primary-600 underline hover:text-primary-800"
		>
			Deselect
		</button>
		<div class="ml-auto flex items-center gap-2">
			{@render children()}
		</div>
	</div>
{/if}
