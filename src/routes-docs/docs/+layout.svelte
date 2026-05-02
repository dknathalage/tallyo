<script lang="ts">
	import { base } from '$app/paths';
	import DocsNavbar from '$lib/components/docs/DocsNavbar.svelte';
	import DocsSidebar from '$lib/components/docs/DocsSidebar.svelte';
	import Logo from '$lib/components/shared/Logo.svelte';
	import type { Snippet } from 'svelte';

	const { children }: { children: Snippet } = $props();
</script>

<div class="flex min-h-screen flex-col bg-white dark:bg-gray-950">
	<DocsNavbar />
	<div class="mx-auto max-w-7xl flex-1 lg:flex">
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
	<footer class="border-t border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900">
		<div class="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
			<div class="flex flex-col items-center justify-between gap-4 sm:flex-row">
				<div class="flex items-center gap-2">
					<Logo size={24} />
					<span class="text-sm font-medium text-gray-900 dark:text-white">Tallyo</span>
				</div>
				<nav class="flex gap-6" aria-label="Footer navigation">
					<a href={`${base}/docs/getting-started`} class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-white">Getting Started</a>
					<a href={`${base}/docs/features`} class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-white">Features</a>
					<a href={`${base}/docs/guides/invoices`} class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-white">Guides</a>
					<a href={`${base}/console`} class="text-sm text-gray-500 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-white">Open App</a>
				</nav>
			</div>
			<p class="mt-4 text-center text-xs text-gray-400 dark:text-gray-500 sm:text-left">
				Local-first. No servers. Your data stays on your device.
			</p>
		</div>
	</footer>
</div>
