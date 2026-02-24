<script lang="ts">
	let {
		item = $bindable(),
		onremove
	}: {
		item: { description: string; quantity: number; rate: number; amount: number };
		onremove: () => void;
	} = $props();

	function recalculate() {
		item.amount = Math.round(item.quantity * item.rate * 100) / 100;
	}
</script>

<div class="flex items-start gap-3">
	<div class="flex-1">
		<input
			type="text"
			bind:value={item.description}
			placeholder="Description"
			class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		/>
	</div>

	<div class="w-24">
		<input
			type="number"
			bind:value={item.quantity}
			oninput={recalculate}
			min="0"
			step="any"
			placeholder="Qty"
			class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		/>
	</div>

	<div class="w-28">
		<input
			type="number"
			bind:value={item.rate}
			oninput={recalculate}
			min="0"
			step="any"
			placeholder="Rate"
			class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		/>
	</div>

	<div class="flex w-28 items-center justify-end py-2 text-sm font-medium text-gray-900">
		${item.amount.toFixed(2)}
	</div>

	<button
		type="button"
		onclick={onremove}
		class="mt-1.5 cursor-pointer rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600"
		aria-label="Remove line item"
	>
		<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
			<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
		</svg>
	</button>
</div>
