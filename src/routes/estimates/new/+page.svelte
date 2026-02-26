<script lang="ts">
	import EstimateForm from '$lib/components/estimate/EstimateForm.svelte';
	import { createEstimate } from '$lib/db/queries/estimates.js';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	async function handleSubmit(
		data: {
			estimate_number: string;
			client_id: number;
			date: string;
			valid_until: string;
			subtotal: number;
			tax_rate: number;
			tax_amount: number;
			total: number;
			notes: string;
			status: string;
			currency_code: string;
			business_snapshot: string;
			client_snapshot: string;
			payer_snapshot: string;
		},
		lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number }>
	) {
		await createEstimate(data, lineItems);
		goto(`${base}/estimates`);
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<a href="{base}/estimates" class="text-gray-400 transition-colors hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300" aria-label={i18n.t('a11y.backToEstimates')}>
			<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
			</svg>
		</a>
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('estimate.newEstimate')}</h1>
	</div>

	<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
		<EstimateForm onsubmit={handleSubmit} />
	</div>
</div>
