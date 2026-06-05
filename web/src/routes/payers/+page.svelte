<script lang="ts">
	import { onMount } from 'svelte';
	import { payers } from '$lib/stores/payers.svelte';
	import type { Payer } from '$lib/api/types';

	// New-item form fields.
	let newName = $state('');
	let newEmail = $state('');
	let newPhone = $state('');
	let newAddress = $state('');
	let creating = $state(false);
	let formError = $state<string | null>(null);

	// Client-side search (generic store has no query support).
	let search = $state('');
	const filtered = $derived.by<Payer[]>(() => {
		const q = search.trim().toLowerCase();
		if (q === '') return payers.items;
		return payers.items.filter(
			(p) =>
				p.name.toLowerCase().includes(q) ||
				p.email.toLowerCase().includes(q) ||
				p.phone.toLowerCase().includes(q)
		);
	});

	// Inline edit state.
	let editId = $state<number | null>(null);
	let editName = $state('');
	let editEmail = $state('');
	let editPhone = $state('');
	let editAddress = $state('');
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	onMount(() => {
		payers.ensureSubscribed();
		void payers.load();
	});

	async function createPayer(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await payers.crud.create({
				name: newName,
				email: newEmail,
				phone: newPhone,
				address: newAddress,
				metadata: ''
			});
			newName = '';
			newEmail = '';
			newPhone = '';
			newAddress = '';
			await payers.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create payer.';
		} finally {
			creating = false;
		}
	}

	function startEdit(payer: Payer): void {
		rowError = null;
		editId = payer.id;
		editName = payer.name;
		editEmail = payer.email;
		editPhone = payer.phone;
		editAddress = payer.address;
	}

	function cancelEdit(): void {
		editId = null;
		rowError = null;
	}

	async function saveEdit(payer: Payer): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await payers.crud.update(payer.id, {
				name: editName,
				email: editEmail,
				phone: editPhone,
				address: editAddress,
				metadata: payer.metadata
			});
			editId = null;
			await payers.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update payer.';
		} finally {
			busy = false;
		}
	}

	async function removePayer(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await payers.crud.remove(id);
			await payers.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete payer.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<h1 class="mb-1 text-xl font-semibold">Payers</h1>
		<p class="mb-6 text-sm text-gray-500">Manage the payers billed on invoices.</p>

		<form class="flex max-w-3xl flex-wrap items-end gap-3" onsubmit={createPayer}>
			<label class="flex-1">
				<span class="mb-1 block text-sm font-medium">Name</span>
				<input
					type="text"
					bind:value={newName}
					required
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="flex-1">
				<span class="mb-1 block text-sm font-medium">Email</span>
				<input
					type="email"
					bind:value={newEmail}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="flex-1">
				<span class="mb-1 block text-sm font-medium">Phone</span>
				<input
					type="text"
					bind:value={newPhone}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="flex-1">
				<span class="mb-1 block text-sm font-medium">Address</span>
				<input
					type="text"
					bind:value={newAddress}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<button
				type="submit"
				disabled={creating}
				class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{creating ? 'Adding…' : 'Add'}
			</button>
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
				placeholder="Filter by name, email, or phone"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		{#if payers.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if payers.error}
			<p class="text-sm text-red-600">{payers.error}</p>
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
						<th class="px-3 py-2 font-medium">Phone</th>
						<th class="px-3 py-2 font-medium">Address</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as payer (payer.id)}
						<tr class="border-b border-gray-100 last:border-0">
							{#if editId === payer.id}
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
									<input
										type="text"
										bind:value={editPhone}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editAddress}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										disabled={busy}
										onclick={() => saveEdit(payer)}
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
								<td class="px-3 py-2 font-medium">{payer.name}</td>
								<td class="px-3 py-2 text-gray-600">{payer.email || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{payer.phone || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{payer.address || '—'}</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										onclick={() => startEdit(payer)}
										class="mr-2 text-gray-900 hover:underline"
									>
										Edit
									</button>
									<button
										type="button"
										disabled={busy}
										onclick={() => removePayer(payer.id)}
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
								No payers found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
