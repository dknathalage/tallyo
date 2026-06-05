<script lang="ts">
	import { onMount } from 'svelte';
	import { catalog } from '$lib/stores/catalog.svelte';
	import type { CatalogItem } from '$lib/api/types';

	// New-item form fields.
	let newName = $state('');
	let newRate = $state(0);
	let newUnit = $state('');
	let newCategory = $state('');
	let newSku = $state('');
	let creating = $state(false);
	let formError = $state<string | null>(null);

	// Client-side search (generic store has no query support).
	let search = $state('');
	const filtered = $derived.by<CatalogItem[]>(() => {
		const q = search.trim().toLowerCase();
		if (q === '') return catalog.items;
		return catalog.items.filter(
			(c) =>
				c.name.toLowerCase().includes(q) ||
				c.sku.toLowerCase().includes(q) ||
				c.category.toLowerCase().includes(q)
		);
	});

	// Inline edit state.
	let editId = $state<number | null>(null);
	let editName = $state('');
	let editRate = $state(0);
	let editUnit = $state('');
	let editCategory = $state('');
	let editSku = $state('');
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	onMount(() => {
		catalog.ensureSubscribed();
		void catalog.load();
	});

	async function createItem(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await catalog.crud.create({
				name: newName,
				rate: newRate,
				unit: newUnit,
				category: newCategory,
				sku: newSku,
				metadata: ''
			});
			newName = '';
			newRate = 0;
			newUnit = '';
			newCategory = '';
			newSku = '';
			await catalog.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create catalog item.';
		} finally {
			creating = false;
		}
	}

	function startEdit(item: CatalogItem): void {
		rowError = null;
		editId = item.id;
		editName = item.name;
		editRate = item.rate;
		editUnit = item.unit;
		editCategory = item.category;
		editSku = item.sku;
	}

	function cancelEdit(): void {
		editId = null;
		rowError = null;
	}

	async function saveEdit(item: CatalogItem): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await catalog.crud.update(item.id, {
				name: editName,
				rate: editRate,
				unit: editUnit,
				category: editCategory,
				sku: editSku,
				metadata: item.metadata
			});
			editId = null;
			await catalog.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update catalog item.';
		} finally {
			busy = false;
		}
	}

	async function removeItem(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await catalog.crud.remove(id);
			await catalog.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete catalog item.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<div class="mb-6 flex items-start justify-between">
			<div>
				<h1 class="mb-1 text-xl font-semibold">Catalog</h1>
				<p class="text-sm text-gray-500">Manage catalog items used as invoice line items.</p>
			</div>
			<div class="flex items-center gap-2">
				<a
					href="/import"
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
				>
					Import
				</a>
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
			</div>
		</div>

		<form class="grid max-w-3xl grid-cols-2 gap-3" onsubmit={createItem}>
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
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">Category</span>
				<input
					type="text"
					bind:value={newCategory}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="col-span-1">
				<span class="mb-1 block text-sm font-medium">SKU</span>
				<input
					type="text"
					bind:value={newSku}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<div class="col-span-2">
				<button
					type="submit"
					disabled={creating}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{creating ? 'Adding…' : 'Add item'}
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
				placeholder="Filter by name, SKU, or category"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		{#if catalog.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if catalog.error}
			<p class="text-sm text-red-600">{catalog.error}</p>
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
						<th class="px-3 py-2 font-medium">Category</th>
						<th class="px-3 py-2 font-medium">SKU</th>
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
									<input
										type="text"
										bind:value={editCategory}
										class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
									/>
								</td>
								<td class="px-3 py-2">
									<input
										type="text"
										bind:value={editSku}
										class="w-24 rounded border border-gray-300 px-2 py-1 text-sm"
									/>
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
									<button
										type="button"
										onclick={cancelEdit}
										class="text-gray-500 hover:underline"
									>
										Cancel
									</button>
								</td>
							{:else}
								<td class="px-3 py-2 font-medium">{item.name}</td>
								<td class="px-3 py-2 text-gray-600">{item.rate}</td>
								<td class="px-3 py-2 text-gray-600">{item.unit || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{item.category || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{item.sku || '—'}</td>
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
							<td colspan="6" class="px-3 py-6 text-center text-gray-500">
								No catalog items found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
