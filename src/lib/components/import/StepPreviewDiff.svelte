<script lang="ts">
	import Button from '$lib/components/shared/Button.svelte';
	import type { DiffResult } from '$lib/import/diff-catalog.js';

	let {
		diff,
		importMode,
		oncommit,
		loading
	}: {
		diff: DiffResult;
		importMode: 'insert_only' | 'upsert';
		oncommit: () => void;
		loading: boolean;
	} = $props();

	let activeTab: 'new' | 'updated' | 'errors' = $state('new');

	let effectiveUpdated = $derived(importMode === 'upsert' ? diff.updatedItems : []);
	let effectiveUnchanged = $derived(
		importMode === 'upsert' ? diff.unchangedCount : diff.unchangedCount + diff.updatedItems.length
	);
	let importableCount = $derived(diff.newItems.length + effectiveUpdated.length);
</script>

<div class="space-y-4">
	<!-- Summary -->
	<div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
		<div class="rounded-lg bg-gray-50 dark:bg-gray-800 p-3 text-center">
			<div class="text-2xl font-bold text-gray-900 dark:text-white">{diff.summary.total}</div>
			<div class="text-xs text-gray-500 dark:text-gray-400">Total</div>
		</div>
		<div class="rounded-lg bg-green-50 p-3 text-center">
			<div class="text-2xl font-bold text-green-700">{diff.newItems.length}</div>
			<div class="text-xs text-green-600">New</div>
		</div>
		<div class="rounded-lg bg-amber-50 p-3 text-center">
			<div class="text-2xl font-bold text-amber-700">{effectiveUpdated.length}</div>
			<div class="text-xs text-amber-600">Updated</div>
		</div>
		<div class="rounded-lg bg-gray-50 dark:bg-gray-800 p-3 text-center">
			<div class="text-2xl font-bold text-gray-500 dark:text-gray-400">{effectiveUnchanged}</div>
			<div class="text-xs text-gray-400 dark:text-gray-500">Unchanged</div>
		</div>
		<div class="rounded-lg bg-red-50 p-3 text-center">
			<div class="text-2xl font-bold text-red-700">{diff.errorItems.length}</div>
			<div class="text-xs text-red-600">Errors</div>
		</div>
	</div>

	<!-- Tabs -->
	<div class="flex gap-1 border-b border-gray-200 dark:border-gray-700">
		<button
			class="cursor-pointer px-4 py-2 text-sm font-medium transition-colors {activeTab === 'new' ? 'border-b-2 border-primary-500 text-primary-600' : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'}"
			onclick={() => (activeTab = 'new')}
		>
			New ({diff.newItems.length})
		</button>
		<button
			class="cursor-pointer px-4 py-2 text-sm font-medium transition-colors {activeTab === 'updated' ? 'border-b-2 border-primary-500 text-primary-600' : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'}"
			onclick={() => (activeTab = 'updated')}
		>
			Updated ({effectiveUpdated.length})
		</button>
		<button
			class="cursor-pointer px-4 py-2 text-sm font-medium transition-colors {activeTab === 'errors' ? 'border-b-2 border-primary-500 text-primary-600' : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'}"
			onclick={() => (activeTab = 'errors')}
		>
			Errors ({diff.errorItems.length})
		</button>
	</div>

	<!-- Tab content -->
	<div class="max-h-64 overflow-auto rounded-lg border border-gray-200 dark:border-gray-700">
		{#if activeTab === 'new'}
			{#if diff.newItems.length === 0}
				<div class="p-4 text-center text-sm text-gray-500 dark:text-gray-400">No new items to add.</div>
			{:else}
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
					<thead class="sticky top-0 bg-green-50">
						<tr>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-green-700">Name</th>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-green-700">SKU</th>
							<th class="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider text-green-700">Rate</th>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-green-700">Unit</th>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-green-700">Category</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-800">
						{#each diff.newItems.slice(0, 100) as item}
							<tr>
								<td class="px-3 py-1.5 text-gray-900 dark:text-white">{item.name}</td>
								<td class="px-3 py-1.5 text-gray-500 dark:text-gray-400">{item.sku || '-'}</td>
								<td class="px-3 py-1.5 text-right text-gray-900 dark:text-white">${item.rate.toFixed(2)}</td>
								<td class="px-3 py-1.5 text-gray-500 dark:text-gray-400">{item.unit || '-'}</td>
								<td class="px-3 py-1.5 text-gray-500 dark:text-gray-400">{item.category || '-'}</td>
							</tr>
						{/each}
					</tbody>
				</table>
				{#if diff.newItems.length > 100}
					<div class="p-2 text-center text-xs text-gray-400 dark:text-gray-500">
						Showing 100 of {diff.newItems.length} new items
					</div>
				{/if}
			{/if}
		{:else if activeTab === 'updated'}
			{#if effectiveUpdated.length === 0}
				<div class="p-4 text-center text-sm text-gray-500 dark:text-gray-400">
					{#if importMode === 'insert_only'}
						Update mode is disabled. Existing items will be skipped.
					{:else}
						No existing items need updating.
					{/if}
				</div>
			{:else}
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
					<thead class="sticky top-0 bg-amber-50">
						<tr>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-amber-700">Name</th>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-amber-700">SKU</th>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-amber-700">Changes</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-800">
						{#each effectiveUpdated.slice(0, 100) as item}
							<tr>
								<td class="px-3 py-1.5 text-gray-900 dark:text-white">{item.existing.name}</td>
								<td class="px-3 py-1.5 text-gray-500 dark:text-gray-400">{item.existing.sku}</td>
								<td class="px-3 py-1.5">
									{#each item.changes as change}
										<div class="text-xs text-amber-700">{change}</div>
									{/each}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
				{#if effectiveUpdated.length > 100}
					<div class="p-2 text-center text-xs text-gray-400 dark:text-gray-500">
						Showing 100 of {effectiveUpdated.length} updated items
					</div>
				{/if}
			{/if}
		{:else if activeTab === 'errors'}
			{#if diff.errorItems.length === 0}
				<div class="p-4 text-center text-sm text-gray-500 dark:text-gray-400">No errors found.</div>
			{:else}
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
					<thead class="sticky top-0 bg-red-50">
						<tr>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-red-700">Row Data</th>
							<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-red-700">Errors</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-800">
						{#each diff.errorItems.slice(0, 50) as item}
							<tr>
								<td class="px-3 py-1.5 text-gray-700 dark:text-gray-300">
									{item.name || Object.values(item._raw).filter(Boolean).slice(0, 3).join(', ') || '(empty)'}
								</td>
								<td class="px-3 py-1.5">
									{#each item._errors as err}
										<div class="text-xs text-red-600">{err}</div>
									{/each}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		{/if}
	</div>

	<!-- Footer -->
	<div class="flex items-center justify-between border-t border-gray-200 dark:border-gray-700 pt-4">
		<div class="text-sm text-gray-500 dark:text-gray-400">
			{importableCount} item{importableCount !== 1 ? 's' : ''} will be imported
		</div>
		<Button disabled={importableCount === 0 || loading} onclick={oncommit}>
			{#if loading}
				Importing...
			{:else}
				Import {importableCount} Item{importableCount !== 1 ? 's' : ''}
			{/if}
		</Button>
	</div>
</div>
