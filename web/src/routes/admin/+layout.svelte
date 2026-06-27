<script lang="ts">
	import Receipt from '@lucide/svelte/icons/receipt';
	import ShieldCheck from '@lucide/svelte/icons/shield-check';

	let { children } = $props();
</script>

<div class="min-h-screen bg-gray-50">
	<header class="border-b border-gray-200 bg-white">
		<div class="mx-auto flex max-w-6xl items-center gap-3 px-4 py-3">
			<a href="/" class="flex items-center gap-2">
				<span class="flex size-7 items-center justify-center rounded-lg bg-brand-700 text-onbrand">
					<Receipt class="size-4" aria-hidden="true" />
				</span>
				<span class="text-lg font-semibold tracking-tight text-gray-900">Tallyo</span>
			</a>
			<span class="text-gray-300">/</span>
			<a href="/admin" class="flex items-center gap-1.5 text-sm font-medium text-gray-600 hover:text-gray-900">
				<ShieldCheck class="size-4" aria-hidden="true" />
				Platform Admin
			</a>
		</div>
	</header>

	<main class="mx-auto w-full max-w-6xl px-4 py-8">
		{@render children()}
	</main>
</div>
