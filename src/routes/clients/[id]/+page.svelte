<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getClient, updateClient, deleteClient } from '$lib/db/queries/clients';
	import { getClientInvoices } from '$lib/db/queries/invoices';
	import { getEntityHistory } from '$lib/db/queries/audit';
	import { getPayer } from '$lib/db/queries/payers';
	import type { AuditLogEntry } from '$lib/types/index.js';
	import ClientForm from '$lib/components/client/ClientForm.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import { formatCurrency, formatDate } from '$lib/utils/format';

	let clientId = $derived(Number(page.params.id));
	let client = $derived(getClient(clientId));
	let invoices = $derived(getClientInvoices(clientId));
	let history = $derived(getEntityHistory('client', clientId));
	let payer = $derived(client?.payer_id ? getPayer(client.payer_id) : null);

	let editing = $state(false);
	let showDeleteConfirm = $state(false);

	function parseMetadataObj(metaStr?: string): Record<string, string> {
		try {
			const obj = JSON.parse(metaStr || '{}');
			return typeof obj === 'object' ? obj : {};
		} catch {
			return {};
		}
	}

	async function handleUpdate(data: { name: string; email: string; phone: string; address: string; metadata: string; payer_id: number | null }) {
		await updateClient(clientId, data);
		editing = false;
	}

	async function handleDelete() {
		await deleteClient(clientId);
		goto(`${base}/clients`);
	}

	function formatTimestamp(ts: string): string {
		const d = new Date(ts + 'Z');
		return d.toLocaleString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	function formatAction(action: string): string {
		return action.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function actionColor(action: string): string {
		if (action === 'create') return 'bg-green-100 text-green-800';
		if (action === 'update') return 'bg-blue-100 text-blue-800';
		if (action === 'delete') return 'bg-red-100 text-red-800';
		return 'bg-gray-100 text-gray-800';
	}

	function parseChanges(changesStr: string): Record<string, { old: unknown; new: unknown }> | null {
		try {
			const parsed = JSON.parse(changesStr);
			if (parsed && typeof parsed === 'object' && Object.keys(parsed).length > 0) {
				return parsed;
			}
			return null;
		} catch {
			return null;
		}
	}

	function formatChangeValue(val: unknown): string {
		if (val === null || val === undefined) return '(empty)';
		if (typeof val === 'number') return String(val);
		return String(val) || '(empty)';
	}
</script>

{#if !client}
	<EmptyState title="Client not found" message="This client does not exist or has been deleted.">
		<a href="{base}/clients">
			<Button variant="secondary">Back to Clients</Button>
		</a>
	</EmptyState>
{:else}
	<div class="space-y-6">
		<!-- Header -->
		<div class="flex items-center justify-between">
			<div>
				<a href="{base}/clients" class="text-sm text-gray-500 hover:text-gray-700">&larr; Back to Clients</a>
				<h1 class="mt-1 text-2xl font-bold text-gray-900">{client.name}</h1>
			</div>
			<div class="flex gap-2">
				{#if !editing}
					<Button variant="secondary" onclick={() => (editing = true)}>Edit</Button>
				{/if}
				<Button variant="danger" onclick={() => (showDeleteConfirm = true)}>Delete</Button>
			</div>
		</div>

		<!-- Client details / Edit form -->
		<div class="rounded-lg border border-gray-200 bg-white p-6">
			{#if editing}
				<h2 class="mb-4 text-lg font-semibold text-gray-900">Edit Client</h2>
				<ClientForm initialData={client} onsubmit={handleUpdate} />
			{:else}
				<dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<dt class="text-sm font-medium text-gray-500">Email</dt>
						<dd class="mt-1 text-sm text-gray-900">{client.email || '-'}</dd>
					</div>
					<div>
						<dt class="text-sm font-medium text-gray-500">Phone</dt>
						<dd class="mt-1 text-sm text-gray-900">{client.phone || '-'}</dd>
					</div>
					<div class="sm:col-span-2">
						<dt class="text-sm font-medium text-gray-500">Address</dt>
						<dd class="mt-1 whitespace-pre-line text-sm text-gray-900">{client.address || '-'}</dd>
					</div>
					{#if Object.keys(parseMetadataObj(client.metadata)).length > 0}
						<div class="sm:col-span-2">
							<dt class="text-sm font-medium text-gray-500">Additional Fields</dt>
							<dd class="mt-1">
								<div class="space-y-1">
									{#each Object.entries(parseMetadataObj(client.metadata)) as [key, value]}
										<div class="text-sm">
											<span class="font-medium text-gray-700">{key}:</span>
											<span class="text-gray-900">{value}</span>
										</div>
									{/each}
								</div>
							</dd>
						</div>
					{/if}
				</dl>
				{#if payer}
					<div class="mt-6 border-t border-gray-200 pt-4">
						<h3 class="text-sm font-medium text-gray-500">Bill-To Payer</h3>
						<div class="mt-2">
							<p class="text-sm font-medium text-gray-900">{payer.name}</p>
							{#if payer.email}<p class="text-sm text-gray-500">{payer.email}</p>{/if}
							{#if payer.phone}<p class="text-sm text-gray-500">{payer.phone}</p>{/if}
							{#if payer.address}<p class="mt-1 whitespace-pre-line text-sm text-gray-500">{payer.address}</p>{/if}
							{#if Object.keys(parseMetadataObj(payer.metadata)).length > 0}
								<div class="mt-2 space-y-1">
									{#each Object.entries(parseMetadataObj(payer.metadata)) as [key, value]}
										<div class="text-sm">
											<span class="font-medium text-gray-700">{key}:</span>
											<span class="text-gray-900">{value}</span>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				{/if}
			{/if}
		</div>

		<!-- Client invoices -->
		<div>
			<div class="flex items-center justify-between">
				<h2 class="text-lg font-semibold text-gray-900">Invoices</h2>
				<a href="{base}/invoices/new?client_id={clientId}">
					<Button size="sm">New Invoice</Button>
				</a>
			</div>

			{#if invoices.length === 0}
				<div class="mt-4">
					<EmptyState title="No invoices" message="This client has no invoices yet." />
				</div>
			{:else}
				<div class="mt-4 overflow-hidden rounded-lg border border-gray-200 bg-white">
					<table class="min-w-full divide-y divide-gray-200">
						<thead class="bg-gray-50">
							<tr>
								<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Invoice #</th>
								<th class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 sm:table-cell">Date</th>
								<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Status</th>
								<th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">Total</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-gray-200">
							{#each invoices as invoice}
								<tr class="transition-colors hover:bg-gray-50">
									<td class="px-6 py-4">
										<a href="{base}/invoices/{invoice.id}" class="font-medium text-primary-600 hover:text-primary-700">
											{invoice.invoice_number}
										</a>
									</td>
									<td class="hidden px-6 py-4 text-sm text-gray-500 sm:table-cell">
										{formatDate(invoice.date)}
									</td>
									<td class="px-6 py-4">
										<StatusBadge status={invoice.status} />
									</td>
									<td class="px-6 py-4 text-right text-sm font-medium text-gray-900">
										{formatCurrency(invoice.total)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>

		<!-- Change History -->
		{#if history.length > 0}
			<div class="rounded-lg border border-gray-200 bg-white p-6">
				<h2 class="mb-4 text-lg font-semibold text-gray-900">Change History</h2>
				<div class="space-y-4">
					{#each history as entry}
						{@const changes = parseChanges(entry.changes)}
						<div class="flex gap-3 border-l-2 border-gray-200 pl-4">
							<div class="flex-1">
								<div class="flex items-center gap-2">
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium {actionColor(entry.action)}">
										{formatAction(entry.action)}
									</span>
									<span class="text-xs text-gray-500">{formatTimestamp(entry.created_at)}</span>
								</div>
								{#if changes}
									<div class="mt-1 space-y-0.5">
										{#each Object.entries(changes) as [field, diff]}
											<p class="text-sm text-gray-600">
												<span class="font-medium">{field}:</span>
												<span class="text-red-600 line-through">{formatChangeValue(diff.old)}</span>
												<span class="text-gray-400">-></span>
												<span class="text-green-700">{formatChangeValue(diff.new)}</span>
											</p>
										{/each}
									</div>
								{:else if entry.context}
									<p class="mt-1 text-sm text-gray-600">{entry.context}</p>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>

	<!-- Delete confirmation -->
	<ConfirmDialog
		open={showDeleteConfirm}
		title="Delete Client"
		message="Are you sure you want to delete {client.name}? This action cannot be undone."
		confirmLabel="Delete"
		confirmVariant="danger"
		onconfirm={handleDelete}
		oncancel={() => (showDeleteConfirm = false)}
	/>
{/if}
