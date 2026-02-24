<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getClient, updateClient, deleteClient } from '$lib/db/queries/clients';
	import { getClientInvoices } from '$lib/db/queries/invoices';
	import ClientForm from '$lib/components/client/ClientForm.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import { formatCurrency, formatDate } from '$lib/utils/format';

	let clientId = $derived(Number(page.params.id));
	let client = $derived(getClient(clientId));
	let invoices = $derived(getClientInvoices(clientId));

	let editing = $state(false);
	let showDeleteConfirm = $state(false);

	async function handleUpdate(data: { name: string; email: string; phone: string; address: string }) {
		await updateClient(clientId, data);
		editing = false;
	}

	async function handleDelete() {
		await deleteClient(clientId);
		goto('/clients');
	}
</script>

{#if !client}
	<EmptyState title="Client not found" message="This client does not exist or has been deleted.">
		<a href="/clients">
			<Button variant="secondary">Back to Clients</Button>
		</a>
	</EmptyState>
{:else}
	<div class="space-y-6">
		<!-- Header -->
		<div class="flex items-center justify-between">
			<div>
				<a href="/clients" class="text-sm text-gray-500 hover:text-gray-700">&larr; Back to Clients</a>
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
				</dl>
			{/if}
		</div>

		<!-- Client invoices -->
		<div>
			<div class="flex items-center justify-between">
				<h2 class="text-lg font-semibold text-gray-900">Invoices</h2>
				<a href="/invoices/new?client_id={clientId}">
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
										<a href="/invoices/{invoice.id}" class="font-medium text-primary-600 hover:text-primary-700">
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
