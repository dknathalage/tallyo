<script lang="ts">
	import { onMount } from 'svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import type { CustomItem } from '$lib/api/types';

	// New-item form fields.
	let newName = $state('');
	let newRate = $state(0);
	let newUnit = $state('');
	let newGstFree = $state(true);
	let creating = $state(false);
	let formError = $state<string | null>(null);
	let showCreate = $state(false);

	function resetNew(): void {
		newName = '';
		newRate = 0;
		newUnit = '';
		newGstFree = true;
	}

	function openCreate(): void {
		resetNew();
		formError = null;
		showCreate = true;
	}

	// Client-side search.
	let search = $state('');
	const filtered = $derived.by<CustomItem[]>(() => {
		const q = search.trim().toLowerCase();
		if (q === '') return customItems.items;
		return customItems.items.filter((c) => c.name.toLowerCase().includes(q));
	});

	// Inline edit state.
	let editId = $state<number | null>(null);
	let editName = $state('');
	let editRate = $state(0);
	let editUnit = $state('');
	let editGstFree = $state(true);
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	onMount(() => {
		customItems.ensureSubscribed();
		void customItems.load();
	});

	async function createItem(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await customItems.crud.create({
				name: newName,
				rate: Number(newRate),
				unit: newUnit,
				gstFree: newGstFree,
				metadata: ''
			});
			resetNew();
			showCreate = false;
			await customItems.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create custom item.';
		} finally {
			creating = false;
		}
	}

	function startEdit(item: CustomItem): void {
		rowError = null;
		editId = item.id;
		editName = item.name;
		editRate = item.rate;
		editUnit = item.unit;
		editGstFree = item.gstFree;
	}

	function cancelEdit(): void {
		editId = null;
		rowError = null;
	}

	async function saveEdit(item: CustomItem): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await customItems.crud.update(item.id, {
				name: editName,
				rate: Number(editRate),
				unit: editUnit,
				gstFree: editGstFree,
				metadata: item.metadata
			});
			editId = null;
			await customItems.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update custom item.';
		} finally {
			busy = false;
		}
	}

	async function removeItem(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await customItems.crud.remove(id);
			await customItems.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete custom item.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<div class="mb-6 flex items-start justify-between">
			<div>
				<h1 class="mb-1 text-xl font-semibold">Custom items</h1>
				<p class="text-sm text-gray-500">
					Your own non-NDIS line items (e.g. travel, gap fees). NDIS support items come from the
					Support catalogue.
				</p>
			</div>
			<div class="flex items-center gap-2">
				<a
					href="/api/export/catalog?format=csv"
					target="_blank"
					rel="noopener"
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
				>
					Export CSV
				</a>
				<a
					href="/api/export/catalog?format=xlsx"
					target="_blank"
					rel="noopener"
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
				>
					Export Excel
				</a>
				<button
					type="button"
					onclick={openCreate}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
				>
					New item
				</button>
			</div>
		</div>

		<Modal bind:open={showCreate} title="New item">
			<form class="grid grid-cols-2 gap-3" onsubmit={createItem}>
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
				<span class="mb-1 block text-sm font-medium">Rate</span>
				<input
					type="number"
					step="0.01"
					bind:value={newRate}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Unit</span>
				<input
					type="text"
					bind:value={newUnit}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1 flex items-end gap-2">
				<input type="checkbox" bind:checked={newGstFree} class="h-4 w-4" />
				<span class="text-sm font-medium">GST-free</span>
			</label>
				{#if formError}
					<p class="col-span-2 text-sm text-red-600">{formError}</p>
				{/if}
				<div class="col-span-2 flex gap-2">
					<button
						type="submit"
						disabled={creating}
						class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
					>
						{creating ? 'Adding…' : 'Add item'}
					</button>
					<button
						type="button"
						onclick={() => (showCreate = false)}
						class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
					>
						Cancel
					</button>
				</div>
			</form>
		</Modal>
	</section>

	<section>
		<label class="mb-4 block max-w-sm">
			<span class="mb-1 block text-sm font-medium">Search</span>
			<input
				type="text"
				bind:value={search}
				placeholder="Filter by name"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		{#if customItems.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if customItems.error}
			<p class="text-sm text-red-600">{customItems.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}

		<div class="overflow-hidden rounded border border-gray-200 bg-white">
			<table class="w-full text-sm">
				<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
					<tr>
						<th class="px-3 py-2 font-medium">Name</th>
						<th class="px-3 py-2 font-medium">Rate</th>
						<th class="px-3 py-2 font-medium">Unit</th>
						<th class="px-3 py-2 font-medium">GST</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as item (item.id)}
						<tr class="border-b border-gray-100 last:border-0">
							{#if editId === item.id}
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editName}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<input
										type="number"
										step="0.01"
										bind:value={editRate}
										class="w-24 rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editUnit}
										class="w-20 rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<label class="flex items-center gap-1">
										<input type="checkbox" bind:checked={editGstFree} class="h-4 w-4" />
										<span class="text-xs">GST-free</span>
									</label>
								</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										disabled={busy}
										onclick={() => saveEdit(item)}
										class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
									>
										Save
									</button>
									<button type="button" onclick={cancelEdit} class="text-gray-500 hover:underline">
										Cancel
									</button>
								</td>
							{:else}
								<td class="px-3 py-2 font-medium">{item.name}</td>
								<td class="px-3 py-2 text-gray-600">{item.rate.toFixed(2)}</td>
								<td class="px-3 py-2 text-gray-600">{item.unit || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{item.gstFree ? 'GST-free' : 'Taxable'}</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										onclick={() => startEdit(item)}
										class="mr-2 text-gray-900 hover:underline"
									>
										Edit
									</button>
									<button
										type="button"
										disabled={busy}
										onclick={() => removeItem(item.id)}
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
								No custom items found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
