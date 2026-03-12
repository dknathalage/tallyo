<script lang="ts">
	import { repositories } from '$lib/repositories';
	import { base } from '$app/paths';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';

	let agingBuckets = $derived(repositories.invoices.getAgingReport());
	let defaultCurrency = $derived(repositories.businessProfile.getBusinessProfile()?.default_currency || 'USD');

	let expandedBuckets = $state<Set<string>>(new Set());

	function toggleBucket(label: string): void {
		if (expandedBuckets.has(label)) {
			expandedBuckets.delete(label);
		} else {
			expandedBuckets.add(label);
		}
		expandedBuckets = new Set(expandedBuckets);
	}

	let totalOutstanding = $derived(agingBuckets.reduce((sum, b) => sum + b.total, 0));
	let totalInvoices = $derived(agingBuckets.reduce((sum, b) => sum + b.invoices.length, 0));

	const bucketColors: Record<string, string> = {
		'Current': 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
		'1–30 days': 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400',
		'31–60 days': 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400',
		'61–90 days': 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
		'90+ days': 'bg-red-200 text-red-900 dark:bg-red-900/50 dark:text-red-300'
	};
</script>

<div class="space-y-6">
	<!-- Header -->
	<div>
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Reports</h1>
		<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">Invoice aging and outstanding balance analysis</p>
	</div>

	<!-- Summary row -->
	<div class="grid grid-cols-2 gap-4 sm:grid-cols-3">
		<div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
			<p class="text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">Total Outstanding</p>
			<p class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{formatCurrency(totalOutstanding, defaultCurrency)}</p>
		</div>
		<div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
			<p class="text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">Unpaid Invoices</p>
			<p class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{totalInvoices}</p>
		</div>
		<div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800 sm:col-span-1 col-span-2">
			<p class="text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">Overdue (31+ days)</p>
			<p class="mt-1 text-2xl font-bold text-red-600 dark:text-red-400">
				{formatCurrency(
					agingBuckets.filter(b => b.label !== 'Current' && b.label !== '1–30 days').reduce((s, b) => s + b.total, 0),
					defaultCurrency
				)}
			</p>
		</div>
	</div>

	<!-- Aging Report -->
	<div class="space-y-4">
		<h2 class="text-lg font-semibold text-gray-900 dark:text-white">Invoice Aging Report</h2>

		{#each agingBuckets as bucket}
			<div class="rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
				<!-- Bucket header (always visible) -->
				<button
					class="flex w-full items-center justify-between px-5 py-4 text-left"
					onclick={() => toggleBucket(bucket.label)}
					aria-expanded={expandedBuckets.has(bucket.label)}
				>
					<div class="flex items-center gap-3">
						<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium {bucketColors[bucket.label] ?? 'bg-gray-100 text-gray-800'}">
							{bucket.label}
						</span>
						<span class="text-sm text-gray-500 dark:text-gray-400">
							{bucket.invoices.length} invoice{bucket.invoices.length === 1 ? '' : 's'}
						</span>
					</div>
					<div class="flex items-center gap-4">
						<span class="text-base font-semibold text-gray-900 dark:text-white">
							{formatCurrency(bucket.total, defaultCurrency)}
						</span>
						<svg
							class="h-4 w-4 text-gray-400 transition-transform {expandedBuckets.has(bucket.label) ? 'rotate-180' : ''}"
							fill="none"
							viewBox="0 0 24 24"
							stroke-width="2"
							stroke="currentColor"
						>
							<path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
						</svg>
					</div>
				</button>

				<!-- Invoice list (expandable) -->
				{#if expandedBuckets.has(bucket.label) && bucket.invoices.length > 0}
					<div class="border-t border-gray-200 dark:border-gray-700">
						<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
							<caption class="sr-only">Invoices in {bucket.label} bucket</caption>
							<thead class="bg-gray-50 dark:bg-gray-900">
								<tr>
									<th class="px-5 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">Invoice</th>
									<th class="hidden px-5 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 sm:table-cell">Client</th>
									<th class="hidden px-5 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 md:table-cell">Due Date</th>
									<th class="px-5 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">Status</th>
									<th class="px-5 py-2 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">Amount</th>
								</tr>
							</thead>
							<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
								{#each bucket.invoices as invoice}
									<tr class="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700">
										<td class="px-5 py-3">
											<a href="{base}/console/invoices/{invoice.id}" class="font-medium text-primary-600 hover:text-primary-700">
												{invoice.invoice_number}
											</a>
										</td>
										<td class="hidden px-5 py-3 text-sm text-gray-500 dark:text-gray-400 sm:table-cell">
											{invoice.client_name ?? '-'}
										</td>
										<td class="hidden px-5 py-3 text-sm text-gray-500 dark:text-gray-400 md:table-cell">
											{formatDate(invoice.due_date)}
										</td>
										<td class="px-5 py-3">
											<StatusBadge status={invoice.status} />
										</td>
										<td class="px-5 py-3 text-right text-sm font-medium text-gray-900 dark:text-white">
											{formatCurrency(invoice.total, invoice.currency_code)}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{:else if expandedBuckets.has(bucket.label) && bucket.invoices.length === 0}
					<div class="border-t border-gray-200 px-5 py-4 text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">
						No invoices in this bucket.
					</div>
				{/if}
			</div>
		{/each}
	</div>
</div>
