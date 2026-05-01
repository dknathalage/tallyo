<script lang="ts">
	import { CURRENCIES } from '$lib/utils/currency.js';

	let {
		value = $bindable(),
		// eslint-disable-next-line prefer-const -- destructure shares a `let` with bindable `value`
		id,
		// eslint-disable-next-line prefer-const -- destructure shares a `let` with bindable `value`
		required = false
	}: {
		value: string;
		id?: string;
		required?: boolean;
	} = $props();
</script>

<select
	{id}
	bind:value
	{required}
	class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
>
	{#each CURRENCIES as currency (currency.code)}
		<option value={currency.code}>{currency.code} — {currency.name}</option>
	{/each}
</select>
