<script lang="ts">
	import { base } from '$app/paths';
	import type { Client } from '$lib/types';

	let { client }: { client: Client } = $props();
</script>

<a
	href="{base}/clients/{client.id}"
	class="block rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 transition-shadow hover:shadow-md"
>
	<h3 class="font-semibold text-gray-900 dark:text-white">{client.name}</h3>
	{#if client.email}
		<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{client.email}</p>
	{/if}
	{#if client.phone}
		<p class="mt-0.5 text-sm text-gray-500 dark:text-gray-400">{client.phone}</p>
	{/if}
</a>
