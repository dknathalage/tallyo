<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getEstimate, getEstimateLineItems, updateEstimate } from '$lib/db/queries/estimates.js';
	import EstimateForm from '$lib/components/estimate/EstimateForm.svelte';
	import type { Estimate, EstimateLineItem } from '$lib/types/index.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let estimate: Estimate | null = $state(null);
	let lineItems: EstimateLineItem[] = $state([]);

	$effect(() => {
		const id = Number(page.params.id);
		const est = getEstimate(id);
		estimate = est;
		if (est) {
			lineItems = getEstimateLineItems(est.id);
		}
	});

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
		items: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number }>
	) {
		if (!estimate) return;
		await updateEstimate(estimate.id, data, items);
		goto(`${base}/estimates/${estimate.id}`);
	}
</script>

{#if !estimate}
	<div class="py-12 text-center">
		<p class="text-gray-500 dark:text-gray-400">{i18n.t('estimate.notFound')}</p>
		<a href="{base}/estimates" class="mt-2 inline-block text-sm text-primary-600 hover:text-primary-700">{i18n.t('estimate.backToEstimates')}</a>
	</div>
{:else}
	<div class="space-y-6">
		<div class="flex items-center gap-3">
			<a href="{base}/estimates/{estimate.id}" class="text-gray-400 transition-colors hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300" aria-label={i18n.t('a11y.backToEstimate')}>
				<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" />
				</svg>
			</a>
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('estimate.editEstimate', { number: estimate.estimate_number })}</h1>
		</div>

		<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
			<EstimateForm initialData={estimate} initialLineItems={lineItems} onsubmit={handleSubmit} />
		</div>
	</div>
{/if}
