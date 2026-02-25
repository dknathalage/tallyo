<script lang="ts">
	import { base } from '$app/paths';
	import { getClients, bulkDeleteClients, updateClient } from '$lib/db/queries/clients';
	import { getRateTiers } from '$lib/db/queries/rate-tiers';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import BulkActionBar from '$lib/components/shared/BulkActionBar.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import ImportExportBar from '$lib/components/csv/ImportExportBar.svelte';
	import ImportPreviewModal from '$lib/components/csv/ImportPreviewModal.svelte';
	import { exportClients } from '$lib/csv/export-clients.js';
	import { parseClientsCsv, commitClientImport } from '$lib/csv/import-clients.js';
	import { CLIENT_COLUMNS } from '$lib/csv/columns.js';
	import type { ParsedImport, CsvClientRow } from '$lib/csv/types.js';

	let search = $state('');
	let showPreview = $state(false);
	let previewData: ParsedImport<CsvClientRow> | null = $state(null);
	let refreshTrigger = $state(0);

	let selectedIds: Set<number> = $state(new Set());
	let showDeleteConfirm = $state(false);

	let tiers = $derived(getRateTiers());

	let clients = $derived.by(() => {
		refreshTrigger;
		return getClients(search || undefined);
	});

	async function handleTierChange(client: { id: number; name: string; email: string; phone: string; address: string }, tierId: number | null) {
		await updateClient(client.id, { name: client.name, email: client.email, phone: client.phone, address: client.address, pricing_tier_id: tierId });
		refreshTrigger++;
	}

	$effect(() => {
		// Clear selection when search changes
		search;
		selectedIds = new Set();
	});

	let allSelected = $derived(clients.length > 0 && selectedIds.size === clients.length);

	function toggleAll() {
		if (allSelected) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(clients.map((c) => c.id));
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
		await bulkDeleteClients([...selectedIds]);
		selectedIds = new Set();
		showDeleteConfirm = false;
		refreshTrigger++;
	}

	async function handleImport(file: File) {
		previewData = await parseClientsCsv(file);
		showPreview = true;
	}

	async function handleConfirm() {
		if (previewData) {
			await commitClientImport(previewData.validRows);
			showPreview = false;
			previewData = null;
			refreshTrigger++;
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Clients</h1>
		<div class="flex items-center gap-3">
			<ImportExportBar onexport={exportClients} onimport={handleImport} />
			<a href="{base}/clients/new">
				<Button>New Client</Button>
			</a>
		</div>
	</div>

	<!-- Search -->
	<div class="max-w-sm">
		<SearchInput bind:value={search} placeholder="Search clients..." />
	</div>

	<!-- Bulk action bar -->
	<BulkActionBar count={selectedIds.size} ondeselect={() => (selectedIds = new Set())}>
		<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>Delete</Button>
	</BulkActionBar>

	<!-- Client list -->
	{#if clients.length === 0}
		{#if search}
			<EmptyState title="No results" message="No clients match your search. Try a different term." />
		{:else}
			<EmptyState title="No clients yet" message="Create your first client to get started.">
				<a href="{base}/clients/new">
					<Button>New Client</Button>
				</a>
			</EmptyState>
		{/if}
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white">
			<table class="min-w-full divide-y divide-gray-200">
				<thead class="bg-gray-50">
					<tr>
						<th class="w-10 px-4 py-3">
							<input
								type="checkbox"
								checked={allSelected}
								onchange={toggleAll}
								class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
							/>
						</th>
						<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Name</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 sm:table-cell">Email</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 md:table-cell">Phone</th>
						<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 lg:table-cell">Tier</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200">
					{#each clients as client}
						<tr class="transition-colors {selectedIds.has(client.id) ? 'bg-primary-50' : 'hover:bg-gray-50'}">
							<td class="w-10 px-4 py-3">
								<input
									type="checkbox"
									checked={selectedIds.has(client.id)}
									onchange={() => toggleOne(client.id)}
									class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
							</td>
							<td class="px-6 py-4">
								<a href="{base}/clients/{client.id}" class="font-medium text-primary-600 hover:text-primary-700">
									{client.name}
								</a>
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 sm:table-cell">
								{client.email || '-'}
							</td>
							<td class="hidden px-6 py-4 text-sm text-gray-500 md:table-cell">
								{client.phone || '-'}
							</td>
							<td class="hidden px-6 py-4 lg:table-cell">
								<select
									value={client.pricing_tier_id ?? ''}
									onchange={(e) => {
										const val = (e.target as HTMLSelectElement).value;
										handleTierChange(client, val ? Number(val) : null);
									}}
									class="rounded border border-gray-300 px-2 py-1 text-xs text-gray-700 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20"
								>
									<option value="">No tier</option>
									{#each tiers as tier}
										<option value={tier.id}>{tier.name}</option>
									{/each}
								</select>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title="Delete Clients">
	<p class="text-sm text-gray-600">
		Are you sure you want to delete {selectedIds.size} client{selectedIds.size === 1 ? '' : 's'}? This action cannot be undone.
	</p>
	<div class="mt-4 flex justify-end gap-3">
		<Button variant="secondary" size="sm" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
		<Button variant="danger" size="sm" onclick={handleBulkDelete}>Delete</Button>
	</div>
</Modal>

{#if previewData}
	<ImportPreviewModal
		open={showPreview}
		onclose={() => { showPreview = false; }}
		onconfirm={handleConfirm}
		title="Import Clients"
		totalRows={previewData.totalRows}
		validRows={previewData.validRows.length}
		skippedDuplicates={previewData.skippedDuplicates}
		errors={previewData.errors}
		columns={[...CLIENT_COLUMNS]}
		previewRows={previewData.validRows}
	/>
{/if}
