<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/nav';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { payers } from '$lib/stores/payers.svelte';
	import type { Payer, PayerInput } from '$lib/api/types';

	const columns: Column<Payer>[] = [
		{ key: 'name', label: 'Name', filter: 'text' },
		{ key: 'email', label: 'Email', filter: 'text' },
		{ key: 'phone', label: 'Phone', filter: 'text' },
		{ key: 'address', label: 'Address', filter: 'text' }
	];

	function toInput(p: Payer): PayerInput {
		return { name: p.name, email: p.email, phone: p.phone, address: p.address, metadata: p.metadata ?? '' };
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	const idParam = $derived((page.params.uuid ?? 'new'));
</script>

{#key idParam}
	<EntityEditor
		title="Payer"
		{columns}
		crud={payers.crud}
		id={idParam}
		{toInput}
		{validate}
		backHref={t('/payers')}
	/>
{/key}
