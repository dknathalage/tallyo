<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		label: string;
		id: string;
		required?: boolean;
		error?: string;
		hint?: string;
		children: Snippet;
	};

	let { label, id, required = false, error, hint, children }: Props = $props();
</script>

<div class="space-y-1">
	<label for={id} class="block text-sm font-medium text-gray-700">
		{label}{#if required}<span class="text-red-600" aria-hidden="true"> *</span>{/if}
	</label>
	{@render children()}
	{#if error}
		<p class="text-xs text-red-600" role="alert">{error}</p>
	{:else if hint}
		<p class="text-xs text-gray-500">{hint}</p>
	{/if}
</div>
