<script lang="ts">
	import { onMount } from 'svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { rateTiers } from '$lib/stores/rateTiers.svelte';
	import { payers } from '$lib/stores/payers.svelte';
	import type { Client } from '$lib/api/types';

	// Selects bind to a string id ('' means none); convert to number | null.
	function toId(v: string): number | null {
		return v === '' ? null : Number(v);
	}

	// New-item form fields.
	let newName = $state('');
	let newEmail = $state('');
	let newPhone = $state('');
	let newAddress = $state('');
	let newPricingTier = $state('');
	let newPayer = $state('');
	let creating = $state(false);
	let formError = $state<string | null>(null);

	// Client-side search (generic store has no query support).
	let search = $state('');
	const filtered = $derived.by<Client[]>(() => {
		const q = search.trim().toLowerCase();
		if (q === '') return clients.items;
		return clients.items.filter(
			(c) => c.name.toLowerCase().includes(q) || c.email.toLowerCase().includes(q)
		);
	});

	// Inline edit state.
	let editId = $state<number | null>(null);
	let editName = $state('');
	let editEmail = $state('');
	let editPhone = $state('');
	let editAddress = $state('');
	let editPricingTier = $state('');
	let editPayer = $state('');
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	onMount(() => {
		clients.ensureSubscribed();
		void clients.load();
		rateTiers.ensureSubscribed();
		void rateTiers.load();
		payers.ensureSubscribed();
		void payers.load();
	});

	async function createClient(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await clients.crud.create({
				name: newName,
				email: newEmail,
				phone: newPhone,
				address: newAddress,
				pricingTierId: toId(newPricingTier),
				metadata: '',
				payerId: toId(newPayer)
			});
			newName = '';
			newEmail = '';
			newPhone = '';
			newAddress = '';
			newPricingTier = '';
			newPayer = '';
			await clients.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create client.';
		} finally {
			creating = false;
		}
	}

	function startEdit(client: Client): void {
		rowError = null;
		editId = client.id;
		editName = client.name;
		editEmail = client.email;
		editPhone = client.phone;
		editAddress = client.address;
		editPricingTier = client.pricingTierId === null ? '' : String(client.pricingTierId);
		editPayer = client.payerId === null ? '' : String(client.payerId);
	}

	function cancelEdit(): void {
		editId = null;
		rowError = null;
	}

	async function saveEdit(client: Client): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await clients.crud.update(client.id, {
				name: editName,
				email: editEmail,
				phone: editPhone,
				address: editAddress,
				pricingTierId: toId(editPricingTier),
				metadata: client.metadata,
				payerId: toId(editPayer)
			});
			editId = null;
			await clients.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update client.';
		} finally {
			busy = false;
		}
	}

	async function removeClient(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await clients.crud.remove(id);
			await clients.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete client.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<h1 class="mb-1 text-xl font-semibold">Clients</h1>
		<p class="mb-6 text-sm text-gray-500">Manage clients billed across invoices.</p>

		<form class="grid max-w-3xl grid-cols-2 gap-3" onsubmit={createClient}>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Name</span>
				<input
					type="text"
					bind:value={newName}
					required
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Email</span>
				<input
					type="email"
					bind:value={newEmail}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Phone</span>
				<input
					type="text"
					bind:value={newPhone}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Address</span>
				<input
					type="text"
					bind:value={newAddress}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Pricing tier</span>
				<select
					bind:value={newPricingTier}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="">— none —</option>
					{#each rateTiers.items as tier (tier.id)}
						<option value={String(tier.id)}>{tier.name}</option>
					{/each}
				</select>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Payer</span>
				<select
					bind:value={newPayer}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="">— none —</option>
					{#each payers.items as payer (payer.id)}
						<option value={String(payer.id)}>{payer.name}</option>
					{/each}
				</select>
			</label>
			<div class="col-span-2">
				<button
					type="submit"
					disabled={creating}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{creating ? 'Adding…' : 'Add client'}
				</button>
			</div>
		</form>

		{#if formError}
			<p class="mt-3 text-sm text-red-600">{formError}</p>
		{/if}
	</section>

	<section>
		<label class="mb-4 block max-w-sm">
			<span class="mb-1 block text-sm font-medium">Search</span>
			<input
				type="text"
				bind:value={search}
				placeholder="Filter by name or email"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		{#if clients.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if clients.error}
			<p class="text-sm text-red-600">{clients.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}

		<div class="overflow-hidden rounded border border-gray-200 bg-white">
			<table class="w-full text-sm">
				<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
					<tr>
						<th class="px-3 py-2 font-medium">Name</th>
						<th class="px-3 py-2 font-medium">Email</th>
						<th class="px-3 py-2 font-medium">Pricing tier</th>
						<th class="px-3 py-2 font-medium">Payer</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as client (client.id)}
						<tr class="border-b border-gray-100 last:border-0">
							{#if editId === client.id}
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editName}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<input
										type="email"
										bind:value={editEmail}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<select
										bind:value={editPricingTier}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									>
										<option value="">— none —</option>
										{#each rateTiers.items as tier (tier.id)}
											<option value={String(tier.id)}>{tier.name}</option>
										{/each}
									</select>
								</td>
								<td class="px-3 py-2">
									<select
										bind:value={editPayer}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									>
										<option value="">— none —</option>
										{#each payers.items as payer (payer.id)}
											<option value={String(payer.id)}>{payer.name}</option>
										{/each}
									</select>
								</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										disabled={busy}
										onclick={() => saveEdit(client)}
										class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
									>
										Save
									</button>
									<button
										type="button"
										onclick={cancelEdit}
										class="text-gray-500 hover:underline"
									>
										Cancel
									</button>
								</td>
							{:else}
								<td class="px-3 py-2 font-medium">{client.name}</td>
								<td class="px-3 py-2 text-gray-600">{client.email || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{client.pricingTierName || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{client.payerName || '—'}</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										onclick={() => startEdit(client)}
										class="mr-2 text-gray-900 hover:underline"
									>
										Edit
									</button>
									<button
										type="button"
										disabled={busy}
										onclick={() => removeClient(client.id)}
										class="text-red-600 hover:underline disabled:opacity-50"
									>
										Delete
									</button>
								</td>
							{/if}
						</tr>
					{:else}
						<tr>
							<td colspan="5" class="px-3 py-6 text-center text-gray-500">
								No clients found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
