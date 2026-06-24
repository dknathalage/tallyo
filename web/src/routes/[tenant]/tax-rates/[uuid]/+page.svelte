<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import type { TaxRate, TaxRateInput } from '$lib/api/types';

	const columns: Column<TaxRate>[] = [
		{ key: 'name', label: 'Name', filter: 'text' },
		{ key: 'rate', label: 'Rate', input: 'number' },
		{ key: 'isDefault', label: 'Default', input: 'checkbox' }
	];

	function toInput(t: TaxRate): TaxRateInput {
		return { name: t.name, rate: t.rate, isDefault: t.isDefault };
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	const idParam = $derived((page.params.uuid ?? 'new'));

	// Creation is modal-only (from the tax-rates list); a stray /tax-rates/new redirects.
	$effect(() => {
		if (idParam === 'new') void goto(t('/tax-rates'));
	});
</script>

{#key idParam}
	{#if idParam !== 'new'}
		<EntityEditor
			title="Tax rate"
			{columns}
			crud={taxRates.crud}
			id={idParam}
			{toInput}
			{validate}
			backHref={t('/tax-rates')}
		/>
	{/if}
{/key}
