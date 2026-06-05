<script lang="ts">
	import { onMount } from 'svelte';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import type { TaxRate } from '$lib/api/types';

	// New-item form fields.
	let newName = $state('');
	let newRate = $state(0);
	let newIsDefault = $state(false);
	let creating = $state(false);
	let formError = $state<string | null>(null);

	// Inline edit state.
	let editId = $state<number | null>(null);
	let editName = $state('');
	let editRate = $state(0);
	let editIsDefault = $state(false);
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	onMount(() => {
		taxRates.ensureSubscribed();
		void taxRates.load();
	});

	async function createTaxRate(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		creating = true;
		try {
			await taxRates.crud.create({
				name: newName,
				rate: newRate,
				isDefault: newIsDefault
			});
			newName = '';
			newRate = 0;
			newIsDefault = false;
			await taxRates.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to create tax rate.';
		} finally {
			creating = false;
		}
	}

	function startEdit(rate: TaxRate): void {
		rowError = null;
		editId = rate.id;
		editName = rate.name;
		editRate = rate.rate;
		editIsDefault = rate.isDefault;
	}

	function cancelEdit(): void {
		editId = null;
		rowError = null;
	}

	async function saveEdit(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await taxRates.crud.update(id, {
				name: editName,
				rate: editRate,
				isDefault: editIsDefault
			});
			editId = null;
			await taxRates.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update tax rate.';
		} finally {
			busy = false;
		}
	}

	async function removeTaxRate(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await taxRates.crud.remove(id);
			await taxRates.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete tax rate.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<h1 class="mb-1 text-xl font-semibold">Tax rates</h1>
		<p class="mb-6 text-sm text-gray-500">Manage the tax rates applied to invoices.</p>

		<form class="flex max-w-2xl flex-wrap items-end gap-3" onsubmit={createTaxRate}>
			<label class="flex-1">
				<span class="mb-1 block text-sm font-medium">Name</span>
				<input
					type="text"
					bind:value={newName}
					required
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="w-32">
				<span class="mb-1 block text-sm font-medium">Rate</span>
				<input
					type="number"
					step="0.01"
					bind:value={newRate}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="flex items-center gap-2 pb-2">
				<input type="checkbox" bind:checked={newIsDefault} class="h-4 w-4" />
				<span class="text-sm font-medium">Default</span>
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
		{#if taxRates.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if taxRates.error}
			<p class="text-sm text-red-600">{taxRates.error}</p>
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
						<th class="px-3 py-2 font-medium">Default</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each taxRates.items as rate (rate.id)}
						<tr class="border-b border-gray-100 last:border-0">
							{#if editId === rate.id}
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
									<input type="checkbox" bind:checked={editIsDefault} class="h-4 w-4" />
								</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										disabled={busy}
										onclick={() => saveEdit(rate.id)}
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
								<td class="px-3 py-2 font-medium">{rate.name}</td>
								<td class="px-3 py-2 text-gray-600">{rate.rate}</td>
								<td class="px-3 py-2">
									{#if rate.isDefault}
										<span
											class="rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800"
										>
											Default
										</span>
									{:else}
										<span class="text-gray-400">—</span>
									{/if}
								</td>
								<td class="px-3 py-2 text-right whitespace-nowrap">
									<button
										type="button"
										onclick={() => startEdit(rate)}
										class="mr-2 text-gray-900 hover:underline"
									>
										Edit
									</button>
									<button
										type="button"
										disabled={busy}
										onclick={() => removeTaxRate(rate.id)}
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
								No tax rates yet.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
