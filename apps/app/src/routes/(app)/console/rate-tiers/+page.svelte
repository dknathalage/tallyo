<script lang="ts">
	import type { RateTier } from '$lib/types';
	import type { PageData } from './$types';
	import Button from '$lib/components/shared/Button.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { invalidateAll } from '$app/navigation';

	let { data }: { data: PageData } = $props();

	// ── Rate Tiers ──────────────────────────────────────────
	let tiers = $derived(data.rateTiers);

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
				await fetch(`/api/rate-tiers/${editingTier.id}`, {
					method: 'PUT',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ name: formName, description: formDescription, sort_order: formSortOrder })
				});
			} else {
				await fetch('/api/rate-tiers', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ name: formName, description: formDescription, sort_order: formSortOrder })
				});
			}
			closeForm();
			await invalidateAll();
		} catch (err) {
			const message = err instanceof Error ? err.message : 'An unexpected error occurred';
			error = message || 'An error occurred';
		}
	}

	function confirmDelete(tier: RateTier) {
		deletingTier = tier;
		showDeleteConfirm = true;
	}

	async function handleDelete() {
		if (!deletingTier) return;
		try {
			await fetch(`/api/rate-tiers/${deletingTier.id}`, { method: 'DELETE' });
			showDeleteConfirm = false;
			deletingTier = null;
			await invalidateAll();
		} catch (err) {
			const message = err instanceof Error ? err.message : 'An unexpected error occurred';
			showDeleteConfirm = false;
			error = message || 'Cannot delete tier';
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('settings.rateTiers')}</h1>
		<Button onclick={openAdd}>{i18n.t('settings.addTier')}</Button>
	</div>

	{#if error && !showForm}
		<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
			{error}
		</div>
	{/if}

	{#if tiers.length === 0}
		<EmptyState title={i18n.t('settings.noTiers')} message={i18n.t('settings.noTiersMessage')}>
			<Button onclick={openAdd}>{i18n.t('settings.addTier')}</Button>
		</EmptyState>
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<caption class="sr-only">{i18n.t('a11y.tiersTable')}</caption>
				<thead class="bg-gray-50 dark:bg-gray-900">
					<tr>
						<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('settings.tierName')}</th>
						<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('settings.description')}</th>
						<th scope="col" class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('settings.sortOrder')}</th>
						<th scope="col" class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('common.actions')}</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each tiers as tier}
						<tr class="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700">
							<td class="px-6 py-4 text-sm font-medium text-gray-900 dark:text-white">{tier.name}</td>
							<td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">{tier.description || '-'}</td>
							<td class="px-6 py-4 text-right text-sm text-gray-500 dark:text-gray-400">{tier.sort_order}</td>
							<td class="px-6 py-4 text-right">
								<div class="flex justify-end gap-2">
									<Button variant="ghost" size="sm" onclick={() => openEdit(tier)}>{i18n.t('common.edit')}</Button>
									<Button
										variant="ghost"
										size="sm"
										onclick={() => confirmDelete(tier)}
										disabled={tiers.length <= 1}
									>
										{i18n.t('common.delete')}
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

<!-- Add/Edit Tier Modal -->
<Modal open={showForm} onclose={closeForm} title={editingTier ? i18n.t('settings.editTier') : i18n.t('settings.addTier')}>
	<form onsubmit={handleSubmit} class="space-y-4">
		{#if error}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
				{error}
			</div>
		{/if}

		<div>
			<label for="tier-name" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.tierName')} <span class="text-red-500">*</span></label>
			<input
				id="tier-name"
				type="text"
				bind:value={formName}
				required
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500"
				placeholder="e.g., Standard, Premium"
			/>
		</div>

		<div>
			<label for="tier-description" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.tierDescription')}</label>
			<input
				id="tier-description"
				type="text"
				bind:value={formDescription}
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500"
				placeholder={i18n.t('settings.tierDescriptionPlaceholder')}
			/>
		</div>

		<div>
			<label for="tier-sort-order" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.tierSortOrder')}</label>
			<input
				id="tier-sort-order"
				type="number"
				min="0"
				step="1"
				bind:value={formSortOrder}
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500"
				placeholder="0"
			/>
		</div>

		<div class="flex justify-end gap-3 pt-2">
			<Button variant="secondary" onclick={closeForm}>{i18n.t('common.cancel')}</Button>
			<Button type="submit">{editingTier ? i18n.t('common.saveChanges') : i18n.t('settings.createTier')}</Button>
		</div>
	</form>
</Modal>

<!-- Delete Tier Confirmation -->
<ConfirmDialog
	open={showDeleteConfirm}
	title={i18n.t('settings.deleteTier')}
	message={i18n.t('settings.deleteTierMessage', { name: deletingTier?.name ?? 'this tier' })}
	confirmLabel={i18n.t('common.delete')}
	confirmVariant="danger"
	onconfirm={handleDelete}
	oncancel={() => { showDeleteConfirm = false; deletingTier = null; }}
/>
