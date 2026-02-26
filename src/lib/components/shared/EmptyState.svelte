<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		message,
		children
	}: {
		title: string;
		message: string;
		children?: Snippet;
	} = $props();
</script>

<div class="flex flex-col items-center justify-center py-12 text-center">
	<!-- Placeholder icon -->
	<div class="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-gray-800">
		<svg class="h-8 w-8 text-gray-400 dark:text-gray-500" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
			<path stroke-linecap="round" stroke-linejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5m6 4.125l2.25 2.25m0 0l2.25 2.25M12 11.625l2.25-2.25M12 11.625l-2.25 2.25M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" />
		</svg>
	</div>

	<h3 class="text-lg font-semibold text-gray-900 dark:text-white">{title}</h3>
	<p class="mt-1 max-w-sm text-sm text-gray-500 dark:text-gray-400">{message}</p>

	{#if children}
		<div class="mt-4">
			{@render children()}
		</div>
	{/if}
</div>
