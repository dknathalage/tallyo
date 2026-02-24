<script lang="ts">
	import { getInvoices } from '$lib/db/queries/invoices.js';
	import type { Invoice } from '$lib/types/index.js';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import Button from '$lib/components/shared/Button.svelte';
	import SearchInput from '$lib/components/shared/SearchInput.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';

	let search = $state('');
	let statusFilter = $state('');
	let invoices: Invoice[] = $state([]);

	const statuses = ['', 'draft', 'sent', 'paid', 'overdue'] as const;
	const statusLabels: Record<string, string> = {
		'': 'All',
		draft: 'Draft',
		sent: 'Sent',
		paid: 'Paid',
		overdue: 'Overdue'
	};

	$effect(() => {
		invoices = getInvoices(search || undefined, statusFilter || undefined);
	});
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Invoices</h1>
		<Button onclick={() => goto(`${base}/invoices/new`)}>New Invoice</Button>
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
							class="cursor-pointer transition-colors hover:bg-gray-50"
							onclick={() => goto(`${base}/invoices/${invoice.id}`)}
						>
							<td class="whitespace-nowrap px-4 py-3 text-sm font-medium text-primary-600">
								{invoice.invoice_number}
							</td>
							<td class="whitespace-nowrap px-4 py-3 text-sm text-gray-900">
								{invoice.client_name ?? 'Unknown'}
							</td>
							<td class="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
								{formatDate(invoice.date)}
							</td>
							<td class="whitespace-nowrap px-4 py-3">
								<StatusBadge status={invoice.status} />
							</td>
							<td class="whitespace-nowrap px-4 py-3 text-right text-sm font-medium text-gray-900">
								{formatCurrency(invoice.total)}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
