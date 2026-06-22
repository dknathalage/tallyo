<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/nav';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { customItems } from '$lib/stores/customItems.svelte';
	import type { CustomItem, CustomItemInput } from '$lib/api/types';

	const columns: Column<CustomItem>[] = [
		{ key: 'name', label: 'Name', filter: 'text' },
		{ key: 'rate', label: 'Rate', input: 'number' },
		{ key: 'unit', label: 'Unit', filter: 'text' },
		{ key: 'gstFree', label: 'GST-free', input: 'checkbox' }
	];

	function toInput(c: CustomItem): CustomItemInput {
		return { name: c.name, rate: Number(c.rate), unit: c.unit, gstFree: c.gstFree, metadata: c.metadata ?? '' };
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	const idParam = $derived((page.params.uuid ?? 'new'));
</script>

{#key idParam}
	<EntityEditor
		title="Custom item"
		{columns}
		crud={customItems.crud}
		id={idParam}
		{toInput}
		{validate}
		blank={{ gstFree: true }}
		backHref={t('/custom-items')}
	/>
{/key}
