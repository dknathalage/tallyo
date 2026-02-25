<script lang="ts">
	import type { KeyValuePair } from '$lib/types';

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
		<div class="flex items-center gap-2">
			<input
				type="text"
				bind:value={pairs[i].key}
				placeholder="Field name"
				{readonly}
				class="w-1/3 rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 {readonly ? 'bg-gray-50' : ''}"
			/>
			<input
				type="text"
				bind:value={pairs[i].value}
				placeholder="Value"
				{readonly}
				class="flex-1 rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 {readonly ? 'bg-gray-50' : ''}"
			/>
			{#if !readonly}
				<button
					type="button"
					onclick={() => removePair(i)}
					class="cursor-pointer rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600"
					aria-label="Remove field"
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
			class="cursor-pointer text-sm font-medium text-primary-600 hover:text-primary-700"
		>
			+ {addLabel}
		</button>
	{/if}
</div>
