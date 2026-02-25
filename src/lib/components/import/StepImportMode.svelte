<script lang="ts">
	import Button from '$lib/components/shared/Button.svelte';

	let {
		onselect
	}: {
		onselect: (mode: 'insert_only' | 'upsert') => void;
	} = $props();

	let mode: 'insert_only' | 'upsert' = $state('upsert');
</script>

<div class="space-y-4">
	<p class="text-sm text-gray-600">Choose how to handle items that already exist in the catalog (matched by SKU).</p>

	<div class="space-y-3">
		<!-- Insert Only -->
		<label
			class="flex cursor-pointer items-start gap-3 rounded-lg border p-4 transition-colors {mode === 'insert_only' ? 'border-primary-500 bg-primary-50' : 'border-gray-200 hover:border-gray-300'}"
		>
			<input
				type="radio"
				bind:group={mode}
				value="insert_only"
				class="mt-0.5 h-4 w-4 cursor-pointer text-primary-600 focus:ring-primary-500"
			/>
			<div>
				<div class="font-medium text-gray-900">Insert only</div>
				<div class="mt-1 text-sm text-gray-500">
					Only add new items to the catalog. Existing items with matching SKUs will be skipped.
				</div>
			</div>
		</label>

		<!-- Upsert -->
		<label
			class="flex cursor-pointer items-start gap-3 rounded-lg border p-4 transition-colors {mode === 'upsert' ? 'border-primary-500 bg-primary-50' : 'border-gray-200 hover:border-gray-300'}"
		>
			<input
				type="radio"
				bind:group={mode}
				value="upsert"
				class="mt-0.5 h-4 w-4 cursor-pointer text-primary-600 focus:ring-primary-500"
			/>
			<div>
				<div class="font-medium text-gray-900">Insert + Update</div>
				<div class="mt-1 text-sm text-gray-500">
					Add new items and update existing ones matched by SKU. Changes will be shown in the preview before committing.
				</div>
			</div>
		</label>
	</div>

	<!-- Footer -->
	<div class="flex justify-end border-t border-gray-200 pt-4">
		<Button onclick={() => onselect(mode)}>
			Next
		</Button>
	</div>
</div>
