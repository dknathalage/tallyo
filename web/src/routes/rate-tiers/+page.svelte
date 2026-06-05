<script lang="ts">
	import { onMount } from 'svelte';
	import { rateTiers } from '$lib/stores/rateTiers.svelte';
	import type { RateTier } from '$lib/api/types';

	// New-item form fields.
	let newName = $state('');
	let newDescription = $state('');
	let newSortOrder = $state(0);
	let creating = $state(false);
	let formError = $state<string | null>(null);

	// Inline edit state.
	let editId = $state<number | null>(null);
	let editName = $state('');
	let editDescription = $state('');
	let editSortOrder = $state(0);
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	onMount(() => {
		rateTiers.ensureSubscribed();
		void rateTiers.load();
	});

	async function createTier(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await rateTiers.crud.create({
				name: newName,
				description: newDescription,
				sortOrder: newSortOrder
			});
			newName = '';
			newDescription = '';
			newSortOrder = 0;
			await rateTiers.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create rate tier.';
		} finally {
			creating = false;
		}
	}

	function startEdit(tier: RateTier): void {
		rowError = null;
		editId = tier.id;
		editName = tier.name;
		editDescription = tier.description;
		editSortOrder = tier.sortOrder;
	}

	function cancelEdit(): void {
		editId = null;
		rowError = null;
	}

	async function saveEdit(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await rateTiers.crud.update(id, {
				name: editName,
				description: editDescription,
				sortOrder: editSortOrder
			});
			editId = null;
			await rateTiers.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update rate tier.';
		} finally {
			busy = false;
		}
	}

	async function removeTier(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await rateTiers.crud.remove(id);
			await rateTiers.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete rate tier.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<h1 class="mb-1 text-xl font-semibold">Rate tiers</h1>
		<p class="mb-6 text-sm text-gray-500">Manage the rate tiers used across invoices.</p>

		<form class="flex max-w-2xl flex-wrap items-end gap-3" onsubmit={createTier}>
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
				<span class="mb-1 block text-sm font-medium">Description</span>
				<input
					type="text"
					bind:value={newDescription}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="w-28">
				<span class="mb-1 block text-sm font-medium">Sort order</span>
				<input
					type="number"
					bind:value={newSortOrder}
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
		{#if rateTiers.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if rateTiers.error}
			<p class="text-sm text-red-600">{rateTiers.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}

		<div class="overflow-hidden rounded border border-gray-200 bg-white">
			<table class="w-full text-sm">
				<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
					<tr>
						<th class="px-3 py-2 font-medium">Name</th>
						<th class="px-3 py-2 font-medium">Description</th>
						<th class="px-3 py-2 font-medium">Sort</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each rateTiers.items as tier (tier.id)}
						<tr class="border-b border-gray-100 last:border-0">
							{#if editId === tier.id}
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editName}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editDescription}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<input
										type="number"
										bind:value={editSortOrder}
										class="w-20 rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										disabled={busy}
										onclick={() => saveEdit(tier.id)}
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
								<td class="px-3 py-2 font-medium">{tier.name}</td>
								<td class="px-3 py-2 text-gray-600">{tier.description || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{tier.sortOrder}</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										onclick={() => startEdit(tier)}
										class="mr-2 text-gray-900 hover:underline"
									>
										Edit
									</button>
									<button
										type="button"
										disabled={busy}
										onclick={() => removeTier(tier.id)}
										class="text-red-600 hover:underline disabled:opacity-50"
									>
										Delete
									</button>
								</td>
							{/if}
						</tr>
					{:else}
						<tr>
							<td colspan="4" class="px-3 py-6 text-center text-gray-500">
								No rate tiers yet.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
