<script lang="ts">
	import DocsNavbar from '$lib/components/docs/DocsNavbar.svelte';
	import DocsSidebar from '$lib/components/docs/DocsSidebar.svelte';

	let { children } = $props();
</script>

<div class="min-h-screen bg-white dark:bg-gray-950">
	<DocsNavbar />
	<div class="mx-auto max-w-7xl lg:flex">
		<aside class="hidden lg:block lg:w-64 lg:shrink-0">
			<div class="sticky top-0 overflow-y-auto p-6">
				<DocsSidebar />
			</div>
		</aside>
		<main class="min-w-0 flex-1 px-6 py-8 lg:px-8">
			<article class="prose dark:prose-invert max-w-none">
				{@render children()}
			</article>
		</main>
	</div>
</div>
