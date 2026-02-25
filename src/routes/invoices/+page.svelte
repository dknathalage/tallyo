<script lang="ts">
	import { getInvoices, bulkDeleteInvoices, bulkUpdateInvoiceStatus } from '$lib/db/queries/invoices.js';
	import type { Invoice } from '$lib/types/index.js';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import Button from '$lib/components/shared/Button.svelte';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import BulkActionBar from '$lib/components/shared/BulkActionBar.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import ImportExportBar from '$lib/components/csv/ImportExportBar.svelte';
	import ImportPreviewModal from '$lib/components/csv/ImportPreviewModal.svelte';
	import { exportInvoices } from '$lib/csv/export-invoices.js';
	import { parseInvoicesCsv, commitInvoiceImport } from '$lib/csv/import-invoices.js';
	import { INVOICE_COLUMNS } from '$lib/csv/columns.js';
	import type { ParsedInvoiceImport } from '$lib/csv/types.js';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';

	let search = $state('');
	let statusFilter = $state('');
	let invoices: Invoice[] = $state([]);
	let showPreview = $state(false);
	let previewData: ParsedInvoiceImport | null = $state(null);
	let importTrigger = $state(0);

	let selectedIds: Set<number> = $state(new Set());
	let showDeleteConfirm = $state(false);

	const statuses = ['', 'draft', 'sent', 'paid', 'overdue'] as const;
	const statusLabels: Record<string, string> = {
		'': 'All',
		draft: 'Draft',
		sent: 'Sent',
		paid: 'Paid',
		overdue: 'Overdue'
	};

	$effect(() => {
		importTrigger;
		invoices = getInvoices(search || undefined, statusFilter || undefined);
		selectedIds = new Set();
	});

	let allSelected = $derived(invoices.length > 0 && selectedIds.size === invoices.length);

	function toggleAll() {
		if (allSelected) {
			selectedIds = new Set();
		} else {
			selectedIds = new Set(invoices.map((i) => i.id));
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
		await bulkDeleteInvoices([...selectedIds]);
		selectedIds = new Set();
		showDeleteConfirm = false;
		importTrigger++;
	}

	async function handleBulkStatus(status: string) {
		await bulkUpdateInvoiceStatus([...selectedIds], status);
		selectedIds = new Set();
		importTrigger++;
	}

	async function handleImport(file: File) {
		previewData = await parseInvoicesCsv(file);
		showPreview = true;
	}

	async function handleConfirm() {
		if (previewData) {
			await commitInvoiceImport(previewData.groups, previewData.newClientsToCreate);
			showPreview = false;
			previewData = null;
			importTrigger++;
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Invoices</h1>
		<div class="flex items-center gap-3">
			<ImportExportBar onexport={exportInvoices} onimport={handleImport} />
			<Button onclick={() => goto(`${base}/invoices/new`)}>New Invoice</Button>
		</div>
	</div>

	<!-- Search and filters -->
	<div class="space-y-3">
		<SearchInput bind:value={search} placeholder="Search invoices..." />

		<div class="flex flex-wrap gap-2">
			{#each statuses as s}
				<button
					onclick={() => (statusFilter = s)}
					class="cursor-pointer rounded-full px-3 py-1 text-sm font-medium transition-colors {statusFilter === s
						? 'bg-primary-600 text-white'
						: 'bg-gray-100 text-gray-600 hover:bg-gray-200'}"
				>
					{statusLabels[s]}
				</button>
			{/each}
		</div>
	</div>

	<!-- Bulk action bar -->
	<BulkActionBar count={selectedIds.size} ondeselect={() => (selectedIds = new Set())}>
		<select
			onchange={(e) => {
				const val = e.currentTarget.value;
				if (val) {
					handleBulkStatus(val);
					e.currentTarget.value = '';
				}
			}}
			class="cursor-pointer rounded-lg border border-gray-300 bg-white px-3 py-1.5 text-sm text-gray-700 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		>
			<option value="">Change status...</option>
			<option value="draft">Draft</option>
			<option value="sent">Sent</option>
			<option value="paid">Paid</option>
			<option value="overdue">Overdue</option>
		</select>
		<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>Delete</Button>
	</BulkActionBar>

	<!-- Invoice list -->
	{#if invoices.length === 0}
		<EmptyState title="No invoices found" message="Create your first invoice to get started.">
			<Button onclick={() => goto(`${base}/invoices/new`)}>New Invoice</Button>
		</EmptyState>
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
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500">Invoice</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500">Client</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500">Date</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500">Status</th>
						<th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500">Total</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200">
					{#each invoices as invoice}
						<tr
							class="transition-colors {selectedIds.has(invoice.id) ? 'bg-primary-50' : 'hover:bg-gray-50'}"
						>
							<td class="w-10 px-4 py-3">
								<input
									type="checkbox"
									checked={selectedIds.has(invoice.id)}
									onchange={() => toggleOne(invoice.id)}
									onclick={(e) => e.stopPropagation()}
									class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm font-medium text-primary-600"
								onclick={() => goto(`${base}/invoices/${invoice.id}`)}
							>
								{invoice.invoice_number}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm text-gray-900"
								onclick={() => goto(`${base}/invoices/${invoice.id}`)}
							>
								{invoice.client_name ?? 'Unknown'}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-sm text-gray-500"
								onclick={() => goto(`${base}/invoices/${invoice.id}`)}
							>
								{formatDate(invoice.date)}
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3"
								onclick={() => goto(`${base}/invoices/${invoice.id}`)}
							>
								<StatusBadge status={invoice.status} />
							</td>
							<td
								class="cursor-pointer whitespace-nowrap px-4 py-3 text-right text-sm font-medium text-gray-900"
								onclick={() => goto(`${base}/invoices/${invoice.id}`)}
							>
								{formatCurrency(invoice.total)}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title="Delete Invoices">
	<p class="text-sm text-gray-600">
		Are you sure you want to delete {selectedIds.size} invoice{selectedIds.size === 1 ? '' : 's'}? This action cannot be undone.
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
		title="Import Invoices"
		totalRows={previewData.totalRows}
		validRows={previewData.validRows.length}
		skippedDuplicates={previewData.skippedDuplicates}
		errors={previewData.errors}
		columns={[...INVOICE_COLUMNS]}
		previewRows={previewData.validRows}
		newClients={previewData.newClientsToCreate}
	/>
{/if}
