<script lang="ts">
	import { base } from '$app/paths';
	import type { Invoice } from '$lib/types/index.js';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';

	let { invoice }: { invoice: Invoice } = $props();
</script>

<a
	href="{base}/invoices/{invoice.id}"
	class="block rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 transition-shadow hover:shadow-md"
>
	<div class="flex items-center justify-between">
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-3">
				<span class="text-sm font-semibold text-gray-900 dark:text-white">{invoice.invoice_number}</span>
				<StatusBadge status={invoice.status} />
			</div>
			<p class="mt-1 truncate text-sm text-gray-600 dark:text-gray-300">{invoice.client_name ?? 'Unknown Client'}</p>
			<p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{formatDate(invoice.date)}</p>
		</div>
		<div class="ml-4 text-right">
			<span class="text-sm font-semibold text-gray-900 dark:text-white">{formatCurrency(invoice.total)}</span>
		</div>
	</div>
</a>
