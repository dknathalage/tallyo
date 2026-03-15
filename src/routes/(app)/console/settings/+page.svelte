<script lang="ts">
	import type { KeyValuePair } from '$lib/types';
	import type { PageData } from './$types';
	import Button from '$lib/components/shared/Button.svelte';
	import KeyValueEditor from '$lib/components/shared/KeyValueEditor.svelte';
	import LogoUploader from '$lib/components/shared/LogoUploader.svelte';
	import CurrencySelect from '$lib/components/shared/CurrencySelect.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	// backup.ts removed: server-side DB lives at ~/.invoices/invoices.db, no manual backup needed
	function exportDatabase() { alert('To backup, copy ~/.invoices/invoices.db'); }
	async function importDatabase(_file: File): Promise<void> { alert('To restore, replace ~/.invoices/invoices.db'); }

	let { data }: { data: PageData } = $props();

	// ── Business Profile ────────────────────────────────────
	function parseMetadata(metaStr?: string): KeyValuePair[] {
		try {
			const obj = JSON.parse(metaStr || '{}');
			return Object.entries(obj).map(([key, value]) => ({ key, value: String(value) }));
		} catch {
			return [];
		}
	}

	const profile = data.businessProfile;
	let bpName = $state(profile?.name ?? '');
	let bpEmail = $state(profile?.email ?? '');
	let bpPhone = $state(profile?.phone ?? '');
	let bpAddress = $state(profile?.address ?? '');
	let bpLogo = $state(profile?.logo ?? '');
	let bpDefaultCurrency = $state(profile?.default_currency ?? 'USD');
	let bpMetadata: KeyValuePair[] = $state(parseMetadata(profile?.metadata));
	let bpSaving = $state(false);
	let bpError = $state('');

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
			await fetch('/api/settings', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					profile: {
						name: bpName,
						email: bpEmail,
						phone: bpPhone,
						address: bpAddress,
						logo: bpLogo,
						metadata: JSON.stringify(metaObj),
						default_currency: bpDefaultCurrency
					}
				})
			});
		} catch (err) {
			const message = err instanceof Error ? err.message : 'An unexpected error occurred';
			bpError = message || 'Failed to save';
		} finally {
			bpSaving = false;
		}
	}

	// ── AI Assistant ─────────────────────────────────────────
	function getApiKeyFromProfile(): string {
		try {
			const obj = JSON.parse(profile?.metadata ?? '{}');
			return (obj.anthropic_api_key as string) ?? '';
		} catch {
			return '';
		}
	}
	let aiApiKey = $state(getApiKeyFromProfile());
	let aiApiKeyVisible = $state(false);
	let aiSaving = $state(false);
	let aiError = $state('');

	async function saveAiSettings() {
		aiError = '';
		aiSaving = true;
		try {
			const existing: Record<string, string> = {};
			for (const pair of bpMetadata) {
				if (pair.key.trim()) existing[pair.key.trim()] = pair.value;
			}
			if (aiApiKey.trim()) {
				existing.anthropic_api_key = aiApiKey.trim();
			} else {
				delete existing.anthropic_api_key;
			}
			await fetch('/api/settings', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					profile: {
						name: bpName,
						email: bpEmail,
						phone: bpPhone,
						address: bpAddress,
						logo: bpLogo,
						metadata: JSON.stringify(existing),
						default_currency: bpDefaultCurrency
					}
				})
			});
		} catch (err) {
			const message = err instanceof Error ? err.message : 'An unexpected error occurred';
			aiError = message || 'Failed to save';
		} finally {
			aiSaving = false;
		}
	}

	// ── Database Backup ─────────────────────────────────────
	let restoreFile: File | null = $state(null);
	let restoreConfirmOpen = $state(false);
	let restoring = $state(false);
	let restoreError = $state('');

	function handleExport() {
		exportDatabase();
	}

	function handleRestoreFileChange(e: Event) {
		const input = e.target as HTMLInputElement;
		restoreFile = input.files?.[0] ?? null;
		restoreError = '';
		if (restoreFile) {
			restoreConfirmOpen = true;
		}
	}

	async function confirmRestore() {
		if (!restoreFile) return;
		restoreConfirmOpen = false;
		restoring = true;
		restoreError = '';
		try {
			await importDatabase(restoreFile);
		} catch (err) {
			const message = err instanceof Error ? err.message : 'An unexpected error occurred';
			restoreError = message || i18n.t('settings.restoreError');
			restoring = false;
		}
	}

	function cancelRestore() {
		restoreConfirmOpen = false;
		restoreFile = null;
		restoreError = '';
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

	<!-- AI Assistant Section -->
	<div class="space-y-4">
		<div>
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white">AI Assistant</h2>
			<p class="text-sm text-gray-500 dark:text-gray-400">Configure your Anthropic API key to enable AI-powered chat.</p>
		</div>

		{#if aiError}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">
				{aiError}
			</div>
		{/if}

		<div class="rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800 p-6">
			<div class="space-y-4">
				<div>
					<label for="ai-api-key" class="block text-sm font-medium text-gray-700 dark:text-gray-300">Anthropic API Key</label>
					<div class="mt-1 flex gap-2">
						<input
							id="ai-api-key"
							type={aiApiKeyVisible ? 'text' : 'password'}
							bind:value={aiApiKey}
							class="block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500"
							placeholder="sk-ant-..."
						/>
						<button
							type="button"
							onclick={() => (aiApiKeyVisible = !aiApiKeyVisible)}
							class="shrink-0 rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-600 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
							aria-label={aiApiKeyVisible ? 'Hide API key' : 'Show API key'}
						>
							{#if aiApiKeyVisible}
								<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" d="M3.98 8.223A10.477 10.477 0 001.934 12C3.226 16.338 7.244 19.5 12 19.5c.993 0 1.953-.138 2.863-.395M6.228 6.228A10.45 10.45 0 0112 4.5c4.756 0 8.773 3.162 10.065 7.498a10.523 10.523 0 01-4.293 5.774M6.228 6.228L3 3m3.228 3.228l3.65 3.65m7.894 7.894L21 21m-3.228-3.228l-3.65-3.65m0 0a3 3 0 10-4.243-4.243m4.242 4.242L9.88 9.88" />
								</svg>
							{:else}
								<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" d="M2.036 12.322a1.012 1.012 0 010-.639C3.423 7.51 7.36 4.5 12 4.5c4.638 0 8.573 3.007 9.963 7.178.07.207.07.431 0 .639C20.577 16.49 16.64 19.5 12 19.5c-4.638 0-8.573-3.007-9.963-7.178z" />
									<path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
								</svg>
							{/if}
						</button>
					</div>
					<p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">Your key is stored locally. Get one at <a href="https://console.anthropic.com" target="_blank" rel="noopener noreferrer" class="underline hover:text-gray-700 dark:hover:text-gray-200">console.anthropic.com</a></p>
				</div>

				<div class="flex justify-end pt-2">
					<Button onclick={saveAiSettings} disabled={aiSaving}>
						{aiSaving ? i18n.t('settings.saving') : i18n.t('settings.saveProfile')}
					</Button>
				</div>
			</div>
		</div>
	</div>

	<!-- Database Backup Section -->
	<div class="space-y-4">
		<div>
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('settings.backup')}</h2>
			<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('settings.backupDesc')}</p>
		</div>

		<div class="rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800 p-6">
			<div class="flex flex-col gap-4 sm:flex-row sm:items-start">
				<!-- Export -->
				<div class="flex-1">
					<h3 class="text-sm font-medium text-gray-900 dark:text-white">{i18n.t('settings.downloadBackup')}</h3>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Save a copy of your database as a <code>.sqlite</code> file.</p>
					<div class="mt-3">
						<Button onclick={handleExport} variant="secondary">
							<svg class="mr-1.5 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
								<path stroke-linecap="round" stroke-linejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5M16.5 12L12 16.5m0 0L7.5 12m4.5 4.5V3" />
							</svg>
							{i18n.t('settings.downloadBackup')}
						</Button>
					</div>
				</div>

				<div class="hidden sm:block w-px bg-gray-200 dark:bg-gray-700 self-stretch"></div>

				<!-- Import -->
				<div class="flex-1">
					<h3 class="text-sm font-medium text-gray-900 dark:text-white">{i18n.t('settings.restoreBackup')}</h3>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Upload a <code>.sqlite</code> backup file to restore your data. <strong class="text-red-600 dark:text-red-400">This overwrites all current data.</strong></p>

					{#if restoreError}
						<div class="mt-2 rounded-lg border border-red-200 bg-red-50 p-2 text-xs text-red-700 dark:border-red-800 dark:bg-red-900/30 dark:text-red-400">
							{restoreError}
						</div>
					{/if}

					<div class="mt-3">
						{#if restoring}
							<p class="text-sm text-gray-500 dark:text-gray-400">{i18n.t('settings.restoring')}</p>
						{:else}
							<label
								class="inline-flex cursor-pointer items-center gap-2 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 shadow-sm transition-colors hover:bg-gray-50 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600"
							>
								<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
								</svg>
								{i18n.t('settings.restoreBackup')}
								<input
									type="file"
									accept=".sqlite,.db"
									class="sr-only"
									onchange={handleRestoreFileChange}
								/>
							</label>
						{/if}
					</div>
				</div>
			</div>
		</div>
	</div>
</div>

<!-- Restore confirmation dialog -->
<ConfirmDialog
	open={restoreConfirmOpen}
	title={i18n.t('settings.restoreConfirmTitle')}
	message={i18n.t('settings.restoreConfirmMessage')}
	confirmLabel={i18n.t('settings.restoreConfirm')}
	confirmVariant="danger"
	onconfirm={confirmRestore}
	oncancel={cancelRestore}
/>
