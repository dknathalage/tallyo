<script lang="ts">
	import { repositories } from '$lib/repositories';
		import { base } from '$app/paths';
	import { formatCurrency, formatDate } from '$lib/utils/format';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let stats = $derived(repositories.dashboard.getDashboardStats());
	let defaultCurrency = $derived(repositories.businessProfile.getBusinessProfile()?.default_currency || 'USD');
</script>

<div class="space-y-6">
	<!-- Header with quick actions -->
	<div class="flex flex-wrap items-center justify-between gap-4">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('dashboard.title')}</h1>
		<div class="flex flex-wrap gap-2">
			<a href="{base}/console/invoices/new">
				<Button>{i18n.t('dashboard.newInvoice')}</Button>
			</a>
			<a href="{base}/console/estimates/new">
				<Button variant="secondary">{i18n.t('dashboard.newEstimate')}</Button>
			</a>
			<a href="{base}/console/clients/new">
				<Button variant="secondary">{i18n.t('dashboard.newClient')}</Button>
			</a>
		</div>
	</div>

	<!-- Stats cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-5">
		<!-- Total Revenue -->
		<div class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
			<div class="flex items-center gap-3">
				<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-green-100 dark:bg-green-900/30">
					<svg class="h-5 w-5 text-green-600 dark:text-green-400" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 6v12m-3-2.818l.879.659c1.171.879 3.07.879 4.242 0 1.172-.879 1.172-2.303 0-3.182C13.536 12.219 12.768 12 12 12c-.725 0-1.45-.22-2.003-.659-1.106-.879-1.106-2.303 0-3.182s2.9-.879 4.006 0l.415.33M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
				</div>
				<div>
					<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('dashboard.totalRevenue')}</p>
					<p class="text-xl font-bold text-gray-900 dark:text-white">{formatCurrency(stats.total_revenue, defaultCurrency)}</p>
				</div>
			</div>
		</div>

		<!-- Outstanding -->
		<div class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
			<div class="flex items-center gap-3">
				<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-100 dark:bg-blue-900/30">
					<svg class="h-5 w-5 text-blue-600 dark:text-blue-400" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
				</div>
				<div>
					<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('dashboard.outstanding')}</p>
					<p class="text-xl font-bold text-gray-900 dark:text-white">{formatCurrency(stats.outstanding_amount, defaultCurrency)}</p>
				</div>
			</div>
		</div>

		<!-- Overdue -->
		<div class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
			<div class="flex items-center gap-3">
				<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-red-100 dark:bg-red-900/30">
					<svg class="h-5 w-5 text-red-600 dark:text-red-400" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
					</svg>
				</div>
				<div>
					<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('dashboard.overdue')}</p>
					<p class="text-xl font-bold text-gray-900 dark:text-white">{stats.overdue_count}</p>
				</div>
			</div>
		</div>

		<!-- Total Clients -->
		<div class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
			<div class="flex items-center gap-3">
				<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-purple-100 dark:bg-purple-900/30">
					<svg class="h-5 w-5 text-purple-600 dark:text-purple-400" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
					</svg>
				</div>
				<div>
					<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('dashboard.totalClients')}</p>
					<p class="text-xl font-bold text-gray-900 dark:text-white">{stats.total_clients}</p>
				</div>
			</div>
		</div>

		<!-- Pending Estimates -->
		<div class="rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800">
			<div class="flex items-center gap-3">
				<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-amber-100 dark:bg-amber-900/30">
					<svg class="h-5 w-5 text-amber-600 dark:text-amber-400" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
					</svg>
				</div>
				<div>
					<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('dashboard.pendingEstimates')}</p>
					<p class="text-xl font-bold text-gray-900 dark:text-white">{stats.pending_estimates}</p>
				</div>
			</div>
		</div>
	</div>

	{#if stats.excluded_currency_count > 0}
		<p class="text-xs text-gray-500 dark:text-gray-400">
			{i18n.t('dashboard.excludedCurrencyNote', { count: stats.excluded_currency_count, plural: stats.excluded_currency_count === 1 ? '' : 's' })}
		</p>
	{/if}

	<!-- Recent Invoices -->
	<div>
		<div class="flex items-center justify-between">
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('dashboard.recentInvoices')}</h2>
			{#if stats.total_invoices > 0}
				<a href="{base}/console/invoices" class="text-sm text-primary-600 hover:text-primary-700">{i18n.t('dashboard.viewAll')}</a>
			{/if}
		</div>

		{#if stats.recent_invoices.length === 0}
			<div class="mt-4">
				<EmptyState title={i18n.t('dashboard.noInvoicesYet')} message={i18n.t('dashboard.noInvoicesMessage')}>
					<a href="{base}/console/invoices/new">
						<Button>{i18n.t('dashboard.repositories.invoices.createInvoice')}</Button>
					</a>
				</EmptyState>
			</div>
		{:else}
			<div class="mt-4 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
					<caption class="sr-only">{i18n.t('a11y.recentInvoicesTable')}</caption>
					<thead class="bg-gray-50 dark:bg-gray-900">
						<tr>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('dashboard.invoice')}</th>
							<th scope="col" class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 sm:table-cell">{i18n.t('dashboard.client')}</th>
							<th scope="col" class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 md:table-cell">{i18n.t('dashboard.date')}</th>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('dashboard.status')}</th>
							<th scope="col" class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('dashboard.total')}</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
						{#each stats.recent_invoices as invoice}
							<tr class="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700">
								<td class="px-6 py-4">
									<a href="{base}/console/invoices/{invoice.id}" class="font-medium text-primary-600 hover:text-primary-700">
										{invoice.invoice_number}
									</a>
								</td>
								<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 sm:table-cell">
									{invoice.client_name ?? '-'}
								</td>
								<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 md:table-cell">
									{formatDate(invoice.date)}
								</td>
								<td class="px-6 py-4">
									<StatusBadge status={invoice.status} />
								</td>
								<td class="px-6 py-4 text-right text-sm font-medium text-gray-900 dark:text-white">
									{formatCurrency(invoice.total, invoice.currency_code)}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<!-- Recent Estimates -->
	<div>
		<div class="flex items-center justify-between">
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('dashboard.recentEstimates')}</h2>
			{#if stats.total_estimates > 0}
				<a href="{base}/console/estimates" class="text-sm text-primary-600 hover:text-primary-700">{i18n.t('dashboard.viewAll')}</a>
			{/if}
		</div>

		{#if stats.recent_estimates.length === 0}
			<div class="mt-4">
				<EmptyState title={i18n.t('dashboard.noEstimatesYet')} message={i18n.t('dashboard.noEstimatesMessage')}>
					<a href="{base}/console/estimates/new">
						<Button>{i18n.t('dashboard.repositories.estimates.createEstimate')}</Button>
					</a>
				</EmptyState>
			</div>
		{:else}
			<div class="mt-4 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
					<caption class="sr-only">{i18n.t('a11y.recentEstimatesTable')}</caption>
					<thead class="bg-gray-50 dark:bg-gray-900">
						<tr>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('dashboard.estimate')}</th>
							<th scope="col" class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 sm:table-cell">{i18n.t('dashboard.client')}</th>
							<th scope="col" class="hidden px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400 md:table-cell">{i18n.t('dashboard.date')}</th>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('dashboard.status')}</th>
							<th scope="col" class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('dashboard.total')}</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
						{#each stats.recent_estimates as estimate}
							<tr class="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700">
								<td class="px-6 py-4">
									<a href="{base}/console/estimates/{estimate.id}" class="font-medium text-primary-600 hover:text-primary-700">
										{estimate.estimate_number}
									</a>
								</td>
								<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 sm:table-cell">
									{estimate.client_name ?? '-'}
								</td>
								<td class="hidden px-6 py-4 text-sm text-gray-500 dark:text-gray-400 md:table-cell">
									{formatDate(estimate.date)}
								</td>
								<td class="px-6 py-4">
									<StatusBadge status={estimate.status} />
								</td>
								<td class="px-6 py-4 text-right text-sm font-medium text-gray-900 dark:text-white">
									{formatCurrency(estimate.total, estimate.currency_code)}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>
