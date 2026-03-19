<script lang="ts">
	import { base } from '$app/paths';
	import type { PageData } from './$types';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import BulkActionBar from '$lib/components/shared/BulkActionBar.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { invalidateAll } from '$app/navigation';

	let { data }: { data: PageData } = $props();

	let search = $state('');
	let selectedIds: Set<number> = $state(new Set());
	let showDeleteConfirm = $state(false);

	let payers = $derived(
		data.payers.filter(p =>
			!search || p.name.toLowerCase().includes(search.toLowerCase()) || (p.email ?? '').toLowerCase().includes(search.toLowerCase())
		)
	);

	$effect(() => {
		search;
		selectedIds = new Set();
	});

	let allSelected = $derived(payers.length > 0 && selectedIds.size === payers.length);

	function toggleAll() {
		if (allSelected) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(payers.map((p) => p.id));
		}
	}

	function toggleOne(id: number) {
		const next = new Set(selectedIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		selectedIds = next;
	}

	async function handleBulkDelete() {
		await fetch('/api/payers', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'bulk-delete', ids: [...selectedIds] })
		});
		selectedIds = new Set();
		showDeleteConfirm = false;
		await invalidateAll();
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('payer.title')}</h1>
		<a href="{base}/console/payers/new">
			<Button>{i18n.t('payer.newPayer')}</Button>
		</a>
	</div>

	<!-- Search -->
	<div class="max-w-sm">
		<SearchInput bind:value={search} placeholder={i18n.t('payer.searchPlaceholder')} />
	</div>

	<!-- Bulk action bar -->
	<BulkActionBar count={selectedIds.size} ondeselect={() => (selectedIds = new Set())}>
		<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>{i18n.t('common.delete')}</Button>
	</BulkActionBar>

	<!-- Payer list -->
	{#if payers.length === 0}
		{#if search}
			<EmptyState title={i18n.t('common.noResults')} message={i18n.t('payer.noResultsMessage')} />
		{:else}
			<EmptyState title={i18n.t('payer.noPayers')} message={i18n.t('payer.noPayersMessage')}>
				<a href="{base}/console/payers/new">
					<Button>{i18n.t('payer.newPayer')}</Button>
				</a>
			</EmptyState>
		{/if}
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<caption class="sr-only">{i18n.t('a11y.payersTable')}</caption>
				<thead class="bg-gray-50 dark:bg-gray-900">
					<tr>
						<th class="w-10 px-4 py-3">
							<input
								type="checkbox"
								checked={allSelected}
								onchange={toggleAll}
								class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
							/>
						</th>
						<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('client.name')}</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 sm:table-cell">{i18n.t('client.email')}</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 md:table-cell">{i18n.t('client.phone')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each payers as payer}
						<tr class="transition-colors {selectedIds.has(payer.id) ? 'bg-primary-50 dark:bg-primary-900/30' : 'hover:bg-gray-50 dark:hover:bg-gray-700'}">
							<td class="w-10 px-4 py-3">
								<input
									type="checkbox"
									checked={selectedIds.has(payer.id)}
									onchange={() => toggleOne(payer.id)}
									class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
							</td>
							<td class="px-6 py-4">
								<a href="{base}/console/payers/{payer.id}" class="font-medium text-primary-600 hover:text-primary-700">
									{payer.name}
								</a>
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 sm:table-cell">
								{payer.email || '-'}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 md:table-cell">
								{payer.phone || '-'}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title={i18n.t('payer.bulkDeleteTitle')}>
	<p class="text-sm text-gray-600 dark:text-gray-300">
		{i18n.t('payer.bulkDeleteMessage', { count: selectedIds.size, plural: selectedIds.size === 1 ? '' : 's' })}
	</p>
	<div class="mt-4 flex justify-end gap-3">
		<Button variant="secondary" size="sm" onclick={() => (showDeleteConfirm = false)}>{i18n.t('common.cancel')}</Button>
		<Button variant="danger" size="sm" onclick={handleBulkDelete}>{i18n.t('common.delete')}</Button>
	</div>
</Modal>
