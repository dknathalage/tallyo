<script lang="ts">
	import type { Snippet } from 'svelte';
	import Sidebar from './Sidebar.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	const { children }: { children: Snippet } = $props();

	let sidebarOpen = $state(false);
</script>

<div class="min-h-screen bg-gray-50 dark:bg-gray-950">
	<a href="#main-content" class="sr-only focus:fixed focus:left-2 focus:top-2 focus:z-[100] focus:rounded-md focus:bg-primary-600 focus:px-4 focus:py-2 focus:text-white focus:shadow-lg">{i18n.t('a11y.skipToContent')}</a>

	<Sidebar bind:open={sidebarOpen} />

	<!-- Mobile top bar -->
	<div class="sticky top-0 z-30 flex h-16 items-center gap-3 border-b border-gray-200 bg-white px-4 dark:border-gray-700 dark:bg-gray-800 lg:hidden">
		<button
			onclick={() => (sidebarOpen = true)}
			class="cursor-pointer rounded-md p-2 text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-white"
			aria-label={i18n.t('a11y.toggleMenu')}
		>
			<svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
			</svg>
		</button>
		<span class="text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('nav.appName')}</span>
	</div>

	<main id="main-content" tabindex="-1" class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:ml-64 lg:px-8">
		{@render children()}
	</main>
</div>
