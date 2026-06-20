<script lang="ts">
	import { page } from '$app/state';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { planManagers } from '$lib/stores/planManagers.svelte';
	import type { PlanManager, PlanManagerInput } from '$lib/api/types';

	const columns: Column<PlanManager>[] = [
		{ key: 'name', label: 'Name', filter: 'text' },
		{ key: 'email', label: 'Email', filter: 'text' },
		{ key: 'phone', label: 'Phone', filter: 'text' },
		{ key: 'address', label: 'Address', filter: 'text' }
	];

	function toInput(p: PlanManager): PlanManagerInput {
		return { name: p.name, email: p.email, phone: p.phone, address: p.address, metadata: p.metadata ?? '' };
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	const idParam = $derived(page.params.id === 'new' ? 'new' : Number(page.params.id));
</script>

{#key idParam}
	<EntityEditor
		title="Plan manager"
		{columns}
		crud={planManagers.crud}
		id={idParam}
		{toInput}
		{validate}
		backHref="/plan-managers"
	/>
{/key}
