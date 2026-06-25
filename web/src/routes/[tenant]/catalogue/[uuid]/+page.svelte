<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { catalogue } from '$lib/stores/catalogue.svelte';
	import type { CatalogueItem, CatalogueItemInput } from '$lib/api/types';

	const columns: Column<CatalogueItem>[] = [
		{ key: 'name', label: 'Name', filter: 'text' },
		{ key: 'code', label: 'Code', filter: 'text' },
		{ key: 'unitPrice', label: 'Unit price', input: 'number' },
		{ key: 'unit', label: 'Unit', filter: 'text' },
		{ key: 'category', label: 'Category', filter: 'text' },
		{ key: 'taxable', label: 'Taxable', input: 'checkbox' }
	];

	function toInput(c: CatalogueItem): CatalogueItemInput {
		return {
			code: c.code,
			name: c.name,
			unit: c.unit,
			category: c.category,
			unitPrice: Number(c.unitPrice),
			taxable: c.taxable,
			metadata: c.metadata ?? ''
		};
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	const idParam = $derived(page.params.uuid ?? 'new');

	// Creation is modal-only (from the catalogue list); a stray /catalogue/new redirects.
	$effect(() => {
		if (idParam === 'new') void goto(t('/catalogue'));
	});
</script>

{#key idParam}
	{#if idParam !== 'new'}
		<EntityEditor
			title="Catalogue item"
			{columns}
			crud={catalogue.crud}
			id={idParam}
			{toInput}
			{validate}
			blank={{ taxable: false }}
			backHref={t('/catalogue')}
		/>
	{/if}
{/key}
