<script lang="ts">
	let {
		value = $bindable(''),
		placeholder = 'Search...'
	}: {
		value: string;
		placeholder?: string;
	} = $props();
</script>

<div class="relative">
	<!-- Search icon -->
	<svg
		class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
		fill="none"
		viewBox="0 0 24 24"
		stroke-width="1.5"
		stroke="currentColor"
	>
		<path stroke-linecap="round" stroke-linejoin="round" d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z" />
	</svg>

	<input
		type="text"
		bind:value
		{placeholder}
		class="w-full rounded-lg border border-gray-300 py-2 pl-10 pr-9 text-sm text-gray-900 placeholder-gray-400 transition-colors focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
	/>

	<!-- Clear button -->
	{#if value}
		<button
			onclick={() => (value = '')}
			class="absolute right-2 top-1/2 -translate-y-1/2 cursor-pointer rounded p-0.5 text-gray-400 transition-colors hover:text-gray-600"
			aria-label="Clear search"
		>
			<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	{/if}
</div>
