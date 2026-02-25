<script lang="ts">
	import { getRateTiers, createRateTier, updateRateTier, deleteRateTier } from '$lib/db/queries/rate-tiers';
	import type { RateTier } from '$lib/types';
	import Button from '$lib/components/shared/Button.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';

	let refreshTrigger = $state(0);
	let tiers = $derived.by(() => {
		refreshTrigger;
		return getRateTiers();
	});

	let showForm = $state(false);
	let editingTier: RateTier | null = $state(null);
	let showDeleteConfirm = $state(false);
	let deletingTier: RateTier | null = $state(null);
	let error = $state('');

	// Form fields
	let formName = $state('');
	let formDescription = $state('');
	let formSortOrder = $state(0);

	function openAdd() {
		editingTier = null;
		formName = '';
		formDescription = '';
		formSortOrder = tiers.length;
		error = '';
		showForm = true;
	}

	function openEdit(tier: RateTier) {
		editingTier = tier;
		formName = tier.name;
		formDescription = tier.description;
		formSortOrder = tier.sort_order;
		error = '';
		showForm = true;
	}

	function closeForm() {
		showForm = false;
		editingTier = null;
		error = '';
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';

		try {
			if (editingTier) {
				await updateRateTier(editingTier.id, {
					name: formName,
					description: formDescription,
					sort_order: formSortOrder
				});
			} else {
				await createRateTier({
					name: formName,
					description: formDescription,
					sort_order: formSortOrder
				});
			}
			closeForm();
			refreshTrigger++;
		} catch (err: any) {
			error = err.message || 'An error occurred';
		}
	}

	function confirmDelete(tier: RateTier) {
		deletingTier = tier;
		showDeleteConfirm = true;
	}

	async function handleDelete() {
		if (!deletingTier) return;
		try {
			await deleteRateTier(deletingTier.id);
			showDeleteConfirm = false;
			deletingTier = null;
			refreshTrigger++;
		} catch (err: any) {
			showDeleteConfirm = false;
			error = err.message || 'Cannot delete tier';
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Settings</h1>
	</div>

	<!-- Rate Tiers Section -->
	<div class="space-y-4">
		<div class="flex items-center justify-between">
			<div>
				<h2 class="text-lg font-semibold text-gray-900">Rate Tiers</h2>
				<p class="text-sm text-gray-500">Manage pricing tiers for catalog items. Clients can be assigned a tier to get tier-specific rates.</p>
			</div>
			<Button onclick={openAdd}>Add Tier</Button>
		</div>

		{#if error && !showForm}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
				{error}
			</div>
		{/if}

		{#if tiers.length === 0}
			<EmptyState title="No rate tiers" message="Create your first rate tier to set up tier-based pricing for catalog items.">
				<Button onclick={openAdd}>Add Tier</Button>
			</EmptyState>
		{:else}
			<div class="overflow-hidden rounded-lg border border-gray-200 bg-white">
				<table class="min-w-full divide-y divide-gray-200">
					<thead class="bg-gray-50">
						<tr>
							<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Name</th>
							<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Description</th>
							<th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">Sort Order</th>
							<th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">Actions</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200">
						{#each tiers as tier}
							<tr class="transition-colors hover:bg-gray-50">
								<td class="px-6 py-4 text-sm font-medium text-gray-900">{tier.name}</td>
								<td class="px-6 py-4 text-sm text-gray-500">{tier.description || '-'}</td>
								<td class="px-6 py-4 text-right text-sm text-gray-500">{tier.sort_order}</td>
								<td class="px-6 py-4 text-right">
									<div class="flex justify-end gap-2">
										<Button variant="ghost" size="sm" onclick={() => openEdit(tier)}>Edit</Button>
										<Button
											variant="ghost"
											size="sm"
											onclick={() => confirmDelete(tier)}
											disabled={tiers.length <= 1}
										>
											Delete
										</Button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>

<!-- Add/Edit Tier Modal -->
<Modal open={showForm} onclose={closeForm} title={editingTier ? 'Edit Tier' : 'Add Tier'}>
	<form onsubmit={handleSubmit} class="space-y-4">
		{#if error}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
				{error}
			</div>
		{/if}

		<div>
			<label for="tier-name" class="block text-sm font-medium text-gray-700">Name <span class="text-red-500">*</span></label>
			<input
				id="tier-name"
				type="text"
				bind:value={formName}
				required
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder="e.g., Standard, Premium"
			/>
		</div>

		<div>
			<label for="tier-description" class="block text-sm font-medium text-gray-700">Description</label>
			<input
				id="tier-description"
				type="text"
				bind:value={formDescription}
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder="Optional description"
			/>
		</div>

		<div>
			<label for="tier-sort-order" class="block text-sm font-medium text-gray-700">Sort Order</label>
			<input
				id="tier-sort-order"
				type="number"
				min="0"
				step="1"
				bind:value={formSortOrder}
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder="0"
			/>
		</div>

		<div class="flex justify-end gap-3 pt-2">
			<Button variant="secondary" onclick={closeForm}>Cancel</Button>
			<Button type="submit">{editingTier ? 'Save Changes' : 'Create Tier'}</Button>
		</div>
	</form>
</Modal>

<!-- Delete Confirmation -->
<ConfirmDialog
	open={showDeleteConfirm}
	title="Delete Tier"
	message="Are you sure you want to delete {deletingTier?.name ?? 'this tier'}? This will remove all tier-specific rates associated with it."
	confirmLabel="Delete"
	confirmVariant="danger"
	onconfirm={handleDelete}
	oncancel={() => { showDeleteConfirm = false; deletingTier = null; }}
/>
