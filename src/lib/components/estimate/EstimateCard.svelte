<script lang="ts">
	import { base } from '$app/paths';
	import type { Estimate } from '$lib/types/index.js';
	import { formatCurrency, formatDate } from '$lib/utils/format.js';
	import StatusBadge from '$lib/components/shared/StatusBadge.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let { estimate }: { estimate: Estimate } = $props();
</script>

<a
	href="{base}/estimates/{estimate.id}"
	class="block rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 transition-shadow hover:shadow-md"
>
	<div class="flex items-center justify-between">
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-3">
				<span class="text-sm font-semibold text-gray-900 dark:text-white">{estimate.estimate_number}</span>
				<StatusBadge status={estimate.status} />
			</div>
			<p class="mt-1 truncate text-sm text-gray-600 dark:text-gray-300">{estimate.client_name ?? i18n.t('invoice.unknownClient')}</p>
			<p class="mt-0.5 text-xs text-gray-400 dark:text-gray-500">{formatDate(estimate.date)}</p>
		</div>
		<div class="ml-4 text-right">
			<span class="text-sm font-semibold text-gray-900 dark:text-white">{formatCurrency(estimate.total, estimate.currency_code)}</span>
		</div>
	</div>
</a>
