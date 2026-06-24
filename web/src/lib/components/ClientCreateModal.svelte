<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from './Modal.svelte';
	import Button from './Button.svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { payers } from '$lib/stores/payers.svelte';
	import type { Client } from '$lib/api/types';

	type Props = {
		open?: boolean;
		onsaved?: (created: Client) => void;
	};

	let { open = $bindable(false), onsaved }: Props = $props();

	// Payer is a relation (id → name), so this is a bespoke modal rather than the
	// generic column-driven CreateModal.
	const payerOptions = $derived(payers.items);
	onMount(() => {
		payers.ensureSubscribed();
		void payers.load();
	});

	let name = $state('');
	let reference = $state('');
	let payer = $state(''); // '' = none
	let email = $state('');
	let phone = $state('');
	let address = $state('');
	let nameError = $state<string | null>(null);
	let saveError = $state<string | null>(null);
	let saving = $state(false);

	// Modal body mounts lazily — seed before that render (see docs/gotchas.md).
	let lastOpen = false;
	$effect.pre(() => {
		if (open && !lastOpen) seed();
		lastOpen = open;
	});

	function seed(): void {
		name = '';
		reference = '';
		payer = '';
		email = '';
		phone = '';
		address = '';
		nameError = null;
		saveError = null;
	}

	async function save(): Promise<void> {
		nameError = name.trim() === '' ? 'Name is required.' : null;
		if (nameError) return;
		saving = true;
		saveError = null;
		try {
			const created = await clients.crud.create({
				name,
				reference,
				payerId: payer === '' ? null : payer,
				email,
				phone,
				address,
				metadata: ''
			});
			open = false;
			onsaved?.(created);
		} catch (err) {
			saveError = err instanceof Error ? err.message : 'Failed to create client.';
		} finally {
			saving = false;
		}
	}
</script>

<Modal bind:open title="New client">
	{#if saveError}<p class="mb-3 text-sm text-red-600">{saveError}</p>{/if}
	<div class="space-y-4">
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Name</span>
			<input
				type="text"
				bind:value={name}
				class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
			/>
			{#if nameError}<span class="mt-1 block text-xs text-red-600">{nameError}</span>{/if}
		</label>

		<label class="block">
			<span class="mb-1 block text-sm font-medium">Reference</span>
			<input
				type="text"
				bind:value={reference}
				class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		<label class="block">
			<span class="mb-1 block text-sm font-medium">Payer</span>
			<select
				bind:value={payer}
				class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
			>
				<option value="">— none —</option>
				{#each payerOptions as pm (pm.id)}
					<option value={String(pm.id)}>{pm.name}</option>
				{/each}
			</select>
		</label>

		<label class="block">
			<span class="mb-1 block text-sm font-medium">Email</span>
			<input
				type="text"
				bind:value={email}
				class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		<label class="block">
			<span class="mb-1 block text-sm font-medium">Phone</span>
			<input
				type="text"
				bind:value={phone}
				class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		<label class="block">
			<span class="mb-1 block text-sm font-medium">Address</span>
			<textarea
				bind:value={address}
				rows="3"
				class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
			></textarea>
		</label>
	</div>

	{#snippet footer()}
		<Button variant="ghost" onclick={() => (open = false)} disabled={saving}>Cancel</Button>
		<Button onclick={save} loading={saving} disabled={saving}>Create</Button>
	{/snippet}
</Modal>
