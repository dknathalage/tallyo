<script lang="ts">
	import { resolve } from '$app/paths';
	import { SvelteSet } from 'svelte/reactivity';
	import type { PageData } from './$types';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import BulkActionBar from '$lib/components/shared/BulkActionBar.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import Pagination from '$lib/components/shared/Pagination.svelte';
	import ImportExportBar from '$lib/components/csv/ImportExportBar.svelte';
	import ImportPreviewModal from '$lib/components/csv/ImportPreviewModal.svelte';
	import { exportClients } from '$lib/csv/export-clients.js';
	import { parseClientsCsv } from '$lib/csv/import-clients.js';
	import { CLIENT_COLUMNS } from '$lib/csv/columns.js';
	import type { ParsedImport, CsvClientRow } from '$lib/csv/types.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { invalidateAll } from '$app/navigation';

	const { data }: { data: PageData } = $props();

	let search = $state('');
	let showPreview = $state(false);
	let previewData: ParsedImport<CsvClientRow> | null = $state(null);

	const selectedIds = new SvelteSet<number>();
	let showDeleteConfirm = $state(false);

	const tiers = $derived(data.rateTiers);
	const paginationResult = $derived(data.clientsResult);

	const clients = $derived(
		paginationResult.data.filter(c =>
			search === '' || c.name.toLowerCase().includes(search.toLowerCase()) || c.email.toLowerCase().includes(search.toLowerCase())
		)
	);

	async function handleTierChange(client: { id: number; name: string; email: string; phone: string; address: string }, tierId: number | null) {
		await fetch(`/api/clients/${client.id}`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name: client.name, email: client.email, phone: client.phone, address: client.address, pricing_tier_id: tierId })
		});
		await invalidateAll();
	}

	$effect(() => {
		// Clear selection when search changes
		void search;
		selectedIds.clear();
	});

	const allSelected = $derived(clients.length > 0 && selectedIds.size === clients.length);

	function toggleAll() {
		if (allSelected) {
			selectedIds.clear();
		} else {
			selectedIds.clear();
			for (const c of clients) selectedIds.add(c.id);
		}
	}

	function toggleOne(id: number) {
		if (selectedIds.has(id)) {
			selectedIds.delete(id);
		} else {
			selectedIds.add(id);
		}
	}

	async function handleBulkDelete() {
		await fetch('/api/clients', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ action: 'bulk-delete', ids: [...selectedIds] })
		});
		selectedIds.clear();
		showDeleteConfirm = false;
		await invalidateAll();
	}

	async function handleImport(file: File) {
		previewData = await parseClientsCsv(file);
		showPreview = true;
	}

	async function handleConfirm() {
		if (previewData) {
			for (const row of previewData.validRows) {
				await fetch('/api/clients', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(row)
				});
			}
			showPreview = false;
			previewData = null;
			await invalidateAll();
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('client.title')}</h1>
		<div class="flex items-center gap-3">
			<ImportExportBar onexport={exportClients} onimport={handleImport} />
			<a href={resolve('/(app)/console/clients/new')}>
				<Button>{i18n.t('client.newClient')}</Button>
			</a>
		</div>
	</div>

	<!-- Search -->
	<div class="max-w-sm">
		<SearchInput bind:value={search} placeholder={i18n.t('client.searchPlaceholder')} />
	</div>

	<!-- Bulk action bar -->
	<BulkActionBar count={selectedIds.size} ondeselect={() => selectedIds.clear()}>
		<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>{i18n.t('common.delete')}</Button>
	</BulkActionBar>

	<!-- Client list -->
	{#if clients.length === 0}
		{#if search}
			<EmptyState title={i18n.t('common.noResults')} message={i18n.t('client.noResultsMessage')} />
		{:else}
			<EmptyState title={i18n.t('client.noClients')} message={i18n.t('client.noClientsMessage')}>
				<a href={resolve('/(app)/console/clients/new')}>
					<Button>{i18n.t('client.newClient')}</Button>
				</a>
			</EmptyState>
		{/if}
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
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
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 lg:table-cell">{i18n.t('client.tier')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each clients as client (client.id)}
						<tr class="transition-colors {selectedIds.has(client.id) ? 'bg-primary-50 dark:bg-primary-900/30' : 'hover:bg-gray-50 dark:hover:bg-gray-700'}">
							<td class="w-10 px-4 py-3">
								<input
									type="checkbox"
									checked={selectedIds.has(client.id)}
									onchange={() => toggleOne(client.id)}
									class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
							</td>
							<td class="px-6 py-4">
								<a href={resolve('/(app)/console/clients/[id]', { id: String(client.id) })} class="font-medium text-primary-600 hover:text-primary-700">
									{client.name}
								</a>
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 sm:table-cell">
								{client.email !== '' ? client.email : '-'}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 md:table-cell">
								{client.phone !== '' ? client.phone : '-'}
							</td>
							<td class="hidden px-6 py-4 lg:table-cell">
								<select
									value={client.pricing_tier_id ?? ''}
									onchange={(e) => {
										const val = (e.target as HTMLSelectElement).value;
										void handleTierChange(client, val !== '' ? Number(val) : null);
									}}
									class="rounded border border-gray-300 px-2 py-1 text-xs text-gray-700 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
								>
									<option value="">{i18n.t('client.noTier')}</option>
									{#each tiers as tier (tier.id)}
										<option value={tier.id}>{tier.name}</option>
									{/each}
								</select>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<Pagination
			total={paginationResult.total}
			currentPage={paginationResult.page}
			totalPages={paginationResult.totalPages}
			hasNextPage={paginationResult.hasNextPage}
			hasPrevPage={paginationResult.hasPrevPage}
		/>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title={i18n.t('client.bulkDeleteTitle')}>
	<p class="text-sm text-gray-600 dark:text-gray-300">
		{i18n.t('client.bulkDeleteMessage', { count: selectedIds.size, plural: selectedIds.size === 1 ? '' : 's' })}
	</p>
	<div class="mt-4 flex justify-end gap-3">
		<Button variant="secondary" size="sm" onclick={() => (showDeleteConfirm = false)}>{i18n.t('common.cancel')}</Button>
		<Button variant="danger" size="sm" onclick={handleBulkDelete}>{i18n.t('common.delete')}</Button>
	</div>
</Modal>

{#if previewData}
	<ImportPreviewModal
		open={showPreview}
		onclose={() => { showPreview = false; }}
		onconfirm={handleConfirm}
		title={i18n.t('client.importTitle')}
		totalRows={previewData.totalRows}
		validRows={previewData.validRows.length}
		skippedDuplicates={previewData.skippedDuplicates}
		errors={previewData.errors}
		columns={[...CLIENT_COLUMNS]}
		previewRows={previewData.validRows}
	/>
{/if}
