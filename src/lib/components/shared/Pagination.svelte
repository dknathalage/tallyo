<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	const {
		total,
		currentPage,
		totalPages,
		hasNextPage,
		hasPrevPage
	}: {
		total: number;
		currentPage: number;
		totalPages: number;
		hasNextPage: boolean;
		hasPrevPage: boolean;
	} = $props();

	function navigate(newPage: number) {
		const url = new URL(page.url);
		url.searchParams.set('page', String(newPage));
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- target is the current page URL with a single query param change; resolve() expects a static route ID and cannot represent runtime query strings
		void goto(url.toString(), { keepFocus: true, noScroll: true });
	}
</script>

{#if totalPages > 1}
	<nav class="flex items-center justify-between border-t border-gray-200 px-1 py-3 dark:border-gray-700" aria-label="Pagination">
		<p class="text-sm text-gray-500 dark:text-gray-400">
			{total} total
		</p>
		<div class="flex items-center gap-2">
			<button
				onclick={() => navigate(currentPage - 1)}
				disabled={!hasPrevPage}
				class="rounded-lg border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
			>
				Prev
			</button>
			<span class="text-sm text-gray-700 dark:text-gray-300">
				Page {currentPage} of {totalPages}
			</span>
			<button
				onclick={() => navigate(currentPage + 1)}
				disabled={!hasNextPage}
				class="rounded-lg border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
			>
				Next
			</button>
		</div>
	</nav>
{/if}
