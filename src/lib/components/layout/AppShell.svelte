<script lang="ts">
	import type { Snippet } from 'svelte';
	import Navbar from './Navbar.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let { children }: { children: Snippet } = $props();
</script>

<div class="min-h-screen bg-gray-50 dark:bg-gray-950">
	<a href="#main-content" class="sr-only focus:fixed focus:left-2 focus:top-2 focus:z-[100] focus:rounded-md focus:bg-primary-600 focus:px-4 focus:py-2 focus:text-white focus:shadow-lg">{i18n.t('a11y.skipToContent')}</a>
	<Navbar />
	<main id="main-content" tabindex="-1" class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
		{@render children()}
	</main>
</div>
