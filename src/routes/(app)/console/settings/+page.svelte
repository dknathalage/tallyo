<script lang="ts">
	import { getBusinessProfile, saveBusinessProfile } from '$lib/db/queries/business-profile';
	import { getPayers, createPayer, updatePayer, deletePayer } from '$lib/db/queries/payers';
	import type { Payer, KeyValuePair } from '$lib/types';
	import Button from '$lib/components/shared/Button.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import KeyValueEditor from '$lib/components/shared/KeyValueEditor.svelte';
	import LogoUploader from '$lib/components/shared/LogoUploader.svelte';
	import CurrencySelect from '$lib/components/shared/CurrencySelect.svelte';
	import PayerForm from '$lib/components/payer/PayerForm.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	// ── Business Profile ────────────────────────────────────
	let bpName = $state('');
	let bpEmail = $state('');
	let bpPhone = $state('');
	let bpAddress = $state('');
	let bpLogo = $state('');
	let bpDefaultCurrency = $state('USD');
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
			bpDefaultCurrency = profile.default_currency || 'USD';
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
				metadata: JSON.stringify(metaObj),
				default_currency: bpDefaultCurrency
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
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('settings.title')}</h1>
	</div>

	<!-- Business Profile Section -->
	<div class="space-y-4">
		<div>
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('settings.businessProfile')}</h2>
			<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('settings.businessProfileDesc')}</p>
		</div>

		{#if bpError}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
				{bpError}
			</div>
		{/if}

		<div class="rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800 p-6">
			<div class="space-y-4">
				<div>
					<label for="bp-name" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.businessName')} <span class="text-red-500">*</span></label>
					<input
						id="bp-name"
						type="text"
						bind:value={bpName}
						required
						class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500"
						placeholder="Your business name"
					/>
				</div>

				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<label for="bp-email" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.email')}</label>
						<input
							id="bp-email"
							type="email"
							bind:value={bpEmail}
							class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500"
							placeholder="business@example.com"
						/>
					</div>
					<div>
						<label for="bp-phone" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.phone')}</label>
						<input
							id="bp-phone"
							type="tel"
							bind:value={bpPhone}
							class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500"
							placeholder="(555) 123-4567"
						/>
					</div>
				</div>

				<div>
					<label for="bp-address" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.address')}</label>
					<textarea
						id="bp-address"
						bind:value={bpAddress}
						rows={3}
						class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500"
						placeholder="Street address, city, state, zip"
					></textarea>
				</div>

				<div>
					<label for="bp-currency" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.defaultCurrency')}</label>
					<div class="mt-1">
						<CurrencySelect id="bp-currency" bind:value={bpDefaultCurrency} />
					</div>
				</div>

				<div>
					<label class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.logo')}</label>
					<div class="mt-1">
						<LogoUploader bind:logo={bpLogo} />
					</div>
				</div>

				<div>
					<label class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('settings.additionalFields')}</label>
					<p class="text-xs text-gray-500 dark:text-gray-400">{i18n.t('settings.additionalFieldsHint')}</p>
					<div class="mt-1">
						<KeyValueEditor bind:pairs={bpMetadata} addLabel={i18n.t('common.addField')} />
					</div>
				</div>

				<div class="flex justify-end pt-2">
					<Button onclick={saveProfile} disabled={bpSaving}>
						{bpSaving ? i18n.t('settings.saving') : i18n.t('settings.saveProfile')}
					</Button>
				</div>
			</div>
		</div>
	</div>

	<!-- Payers Section -->
	<div class="space-y-4">
		<div class="flex items-center justify-between">
			<div>
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('settings.payers')}</h2>
				<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('settings.payersDesc')}</p>
			</div>
			<Button onclick={openAddPayer}>{i18n.t('settings.addPayer')}</Button>
		</div>

		{#if payerError && !showPayerForm}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
				{payerError}
			</div>
		{/if}

		{#if payers.length === 0}
			<EmptyState title={i18n.t('settings.noPayers')} message={i18n.t('settings.noPayersMessage')}>
				<Button onclick={openAddPayer}>{i18n.t('settings.addPayer')}</Button>
			</EmptyState>
		{:else}
			<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
					<caption class="sr-only">{i18n.t('a11y.payersTable')}</caption>
					<thead class="bg-gray-50 dark:bg-gray-900">
						<tr>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('client.name')}</th>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('settings.email')}</th>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('settings.phone')}</th>
							<th scope="col" class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('common.actions')}</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
						{#each payers as payer}
							<tr class="transition-colors hover:bg-gray-50 dark:hover:bg-gray-700">
								<td class="px-6 py-4 text-sm font-medium text-gray-900 dark:text-white">{payer.name}</td>
								<td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">{payer.email || '-'}</td>
								<td class="px-6 py-4 text-sm text-gray-500 dark:text-gray-400">{payer.phone || '-'}</td>
								<td class="px-6 py-4 text-right">
									<div class="flex justify-end gap-2">
										<Button variant="ghost" size="sm" onclick={() => openEditPayer(payer)}>{i18n.t('common.edit')}</Button>
										<Button variant="ghost" size="sm" onclick={() => confirmDeletePayer(payer)}>{i18n.t('common.delete')}</Button>
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

<!-- Add/Edit Payer Modal -->
<Modal open={showPayerForm} onclose={closePayerForm} title={editingPayer ? i18n.t('settings.editPayer') : i18n.t('settings.addPayer')}>
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
	title={i18n.t('settings.deletePayer')}
	message={i18n.t('settings.deletePayerMessage', { name: deletingPayer?.name ?? 'this payer' })}
	confirmLabel={i18n.t('common.delete')}
	confirmVariant="danger"
	onconfirm={handleDeletePayer}
	oncancel={() => { showPayerDeleteConfirm = false; deletingPayer = null; }}
/>
