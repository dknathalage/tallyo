<script lang="ts">
	import { getRateTiers, createRateTier, updateRateTier, deleteRateTier } from '$lib/db/queries/rate-tiers';
	import { getBusinessProfile, saveBusinessProfile } from '$lib/db/queries/business-profile';
	import { getPayers, createPayer, updatePayer, deletePayer } from '$lib/db/queries/payers';
	import type { RateTier, Payer, KeyValuePair } from '$lib/types';
	import Button from '$lib/components/shared/Button.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import KeyValueEditor from '$lib/components/shared/KeyValueEditor.svelte';
	import LogoUploader from '$lib/components/shared/LogoUploader.svelte';
	import PayerForm from '$lib/components/payer/PayerForm.svelte';

	// ── Rate Tiers ──────────────────────────────────────────
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

	// ── Business Profile ────────────────────────────────────
	let bpName = $state('');
	let bpEmail = $state('');
	let bpPhone = $state('');
	let bpAddress = $state('');
	let bpLogo = $state('');
	let bpMetadata: KeyValuePair[] = $state([]);
	let bpSaving = $state(false);
	let bpError = $state('');

	function parseMetadata(metaStr?: string): KeyValuePair[] {
		try {
			const obj = JSON.parse(metaStr || '{}');
			return Object.entries(obj).map(([key, value]) => ({ key, value: String(value) }));
		} catch {
			return [];
		}
	}

	$effect(() => {
		const profile = getBusinessProfile();
		if (profile) {
			bpName = profile.name;
			bpEmail = profile.email;
			bpPhone = profile.phone;
			bpAddress = profile.address;
			bpLogo = profile.logo;
			bpMetadata = parseMetadata(profile.metadata);
		}
	});

	async function saveProfile() {
		bpError = '';
		bpSaving = true;
		try {
			const metaObj: Record<string, string> = {};
			for (const pair of bpMetadata) {
				if (pair.key.trim()) {
					metaObj[pair.key.trim()] = pair.value;
				}
			}
			await saveBusinessProfile({
				name: bpName,
				email: bpEmail,
				phone: bpPhone,
				address: bpAddress,
				logo: bpLogo,
				metadata: JSON.stringify(metaObj)
			});
		} catch (err: any) {
			bpError = err.message || 'Failed to save';
		} finally {
			bpSaving = false;
		}
	}

	// ── Payers ──────────────────────────────────────────────
	let payerRefreshTrigger = $state(0);
	let payers = $derived.by(() => {
		payerRefreshTrigger;
		return getPayers();
	});

	let showPayerForm = $state(false);
	let editingPayer: Payer | null = $state(null);
	let showPayerDeleteConfirm = $state(false);
	let deletingPayer: Payer | null = $state(null);
	let payerError = $state('');

	function openAddPayer() {
		editingPayer = null;
		payerError = '';
		showPayerForm = true;
	}

	function openEditPayer(payer: Payer) {
		editingPayer = payer;
		payerError = '';
		showPayerForm = true;
	}

	function closePayerForm() {
		showPayerForm = false;
		editingPayer = null;
		payerError = '';
	}

	async function handlePayerSubmit(data: { name: string; email: string; phone: string; address: string; metadata: string }) {
		payerError = '';
		try {
			if (editingPayer) {
				await updatePayer(editingPayer.id, data);
			} else {
				await createPayer(data);
			}
			closePayerForm();
			payerRefreshTrigger++;
		} catch (err: any) {
			payerError = err.message || 'An error occurred';
		}
	}

	function confirmDeletePayer(payer: Payer) {
		deletingPayer = payer;
		showPayerDeleteConfirm = true;
	}

	async function handleDeletePayer() {
		if (!deletingPayer) return;
		try {
			await deletePayer(deletingPayer.id);
			showPayerDeleteConfirm = false;
			deletingPayer = null;
			payerRefreshTrigger++;
		} catch (err: any) {
			showPayerDeleteConfirm = false;
			payerError = err.message || 'Cannot delete payer';
		}
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold text-gray-900">Settings</h1>
	</div>

	<!-- Business Profile Section -->
	<div class="space-y-4">
		<div>
			<h2 class="text-lg font-semibold text-gray-900">Business Profile</h2>
			<p class="text-sm text-gray-500">Your business details that appear on invoices.</p>
		</div>

		{#if bpError}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
				{bpError}
			</div>
		{/if}

		<div class="rounded-lg border border-gray-200 bg-white p-6">
			<div class="space-y-4">
				<div>
					<label for="bp-name" class="block text-sm font-medium text-gray-700">Business Name <span class="text-red-500">*</span></label>
					<input
						id="bp-name"
						type="text"
						bind:value={bpName}
						required
						class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
						placeholder="Your business name"
					/>
				</div>

				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<label for="bp-email" class="block text-sm font-medium text-gray-700">Email</label>
						<input
							id="bp-email"
							type="email"
							bind:value={bpEmail}
							class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
							placeholder="business@example.com"
						/>
					</div>
					<div>
						<label for="bp-phone" class="block text-sm font-medium text-gray-700">Phone</label>
						<input
							id="bp-phone"
							type="tel"
							bind:value={bpPhone}
							class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
							placeholder="(555) 123-4567"
						/>
					</div>
				</div>

				<div>
					<label for="bp-address" class="block text-sm font-medium text-gray-700">Address</label>
					<textarea
						id="bp-address"
						bind:value={bpAddress}
						rows={3}
						class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
						placeholder="Street address, city, state, zip"
					></textarea>
				</div>

				<div>
					<label class="block text-sm font-medium text-gray-700">Logo</label>
					<div class="mt-1">
						<LogoUploader bind:logo={bpLogo} />
					</div>
				</div>

				<div>
					<label class="block text-sm font-medium text-gray-700">Additional Fields</label>
					<p class="text-xs text-gray-500">e.g., ABN, registration numbers, tax IDs</p>
					<div class="mt-1">
						<KeyValueEditor bind:pairs={bpMetadata} addLabel="Add Field" />
					</div>
				</div>

				<div class="flex justify-end pt-2">
					<Button onclick={saveProfile} disabled={bpSaving}>
						{bpSaving ? 'Saving...' : 'Save Profile'}
					</Button>
				</div>
			</div>
		</div>
	</div>

	<!-- Payers Section -->
	<div class="space-y-4">
		<div class="flex items-center justify-between">
			<div>
				<h2 class="text-lg font-semibold text-gray-900">Payers</h2>
				<p class="text-sm text-gray-500">Manage payers (bill-to parties) that can be assigned to clients.</p>
			</div>
			<Button onclick={openAddPayer}>Add Payer</Button>
		</div>

		{#if payerError && !showPayerForm}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
				{payerError}
			</div>
		{/if}

		{#if payers.length === 0}
			<EmptyState title="No payers" message="Create your first payer to assign bill-to parties to clients.">
				<Button onclick={openAddPayer}>Add Payer</Button>
			</EmptyState>
		{:else}
			<div class="overflow-hidden rounded-lg border border-gray-200 bg-white">
				<table class="min-w-full divide-y divide-gray-200">
					<thead class="bg-gray-50">
						<tr>
							<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Name</th>
							<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Email</th>
							<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Phone</th>
							<th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">Actions</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200">
						{#each payers as payer}
							<tr class="transition-colors hover:bg-gray-50">
								<td class="px-6 py-4 text-sm font-medium text-gray-900">{payer.name}</td>
								<td class="px-6 py-4 text-sm text-gray-500">{payer.email || '-'}</td>
								<td class="px-6 py-4 text-sm text-gray-500">{payer.phone || '-'}</td>
								<td class="px-6 py-4 text-right">
									<div class="flex justify-end gap-2">
										<Button variant="ghost" size="sm" onclick={() => openEditPayer(payer)}>Edit</Button>
										<Button variant="ghost" size="sm" onclick={() => confirmDeletePayer(payer)}>Delete</Button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
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

<!-- Delete Tier Confirmation -->
<ConfirmDialog
	open={showDeleteConfirm}
	title="Delete Tier"
	message="Are you sure you want to delete {deletingTier?.name ?? 'this tier'}? This will remove all tier-specific rates associated with it."
	confirmLabel="Delete"
	confirmVariant="danger"
	onconfirm={handleDelete}
	oncancel={() => { showDeleteConfirm = false; deletingTier = null; }}
/>

<!-- Add/Edit Payer Modal -->
<Modal open={showPayerForm} onclose={closePayerForm} title={editingPayer ? 'Edit Payer' : 'Add Payer'}>
	{#if payerError}
		<div class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
			{payerError}
		</div>
	{/if}
	<PayerForm initialData={editingPayer ?? undefined} onsubmit={handlePayerSubmit} />
</Modal>

<!-- Delete Payer Confirmation -->
<ConfirmDialog
	open={showPayerDeleteConfirm}
	title="Delete Payer"
	message="Are you sure you want to delete {deletingPayer?.name ?? 'this payer'}? Clients assigned to this payer will need to be updated."
	confirmLabel="Delete"
	confirmVariant="danger"
	onconfirm={handleDeletePayer}
	oncancel={() => { showPayerDeleteConfirm = false; deletingPayer = null; }}
/>
