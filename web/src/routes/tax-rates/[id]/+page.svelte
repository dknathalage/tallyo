<script lang="ts">
	import { page } from '$app/state';
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

	const idParam = $derived(page.params.id === 'new' ? 'new' : Number(page.params.id));
</script>

<EntityEditor
	title="Tax rate"
	{columns}
	crud={taxRates.crud}
	id={idParam}
	{toInput}
	{validate}
	backHref="/tax-rates"
/>
