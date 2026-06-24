<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		children: Snippet;
		padded?: boolean;
		class?: string;
	};

	let { children, padded = true, class: extra = '' }: Props = $props();
</script>

<div
	class="rounded-xl border border-gray-200 bg-white shadow-sm {padded ? 'p-5' : ''} {extra}"
>
	{@render children()}
</div>
