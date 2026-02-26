<script lang="ts">
	import type { Snippet } from 'svelte';
	import Navbar from './Navbar.svelte';

	let { children }: { children: Snippet } = $props();
</script>

<div class="min-h-screen bg-gray-50 dark:bg-gray-950">
	<Navbar />
	<main class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
		{@render children()}
	</main>
</div>
