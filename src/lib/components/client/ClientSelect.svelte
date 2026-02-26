<script lang="ts">
	import type { Client } from '$lib/types';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let {
		value = $bindable(0),
		clients
	}: {
		value: number;
		clients: Client[];
	} = $props();
</script>

<select
	bind:value={value}
	class="block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
>
	<option value={0} disabled>{i18n.t('client.selectClient')}</option>
	{#each clients as client}
		<option value={client.id}>{client.name}</option>
	{/each}
</select>
