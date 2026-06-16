<script lang="ts">
	import { onMount } from 'svelte';
	import { supportCatalog } from '$lib/stores/supportCatalog.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { apiUpload } from '$lib/api/client';
	import type { CatalogVersion, SupportItem, SupportItemPrice } from '$lib/api/types';

	function money(n: number | null): string {
		if (n === null) return 'Quote';
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	function zoneLabel(z: string): string {
		switch (z) {
			case 'very_remote':
				return 'Very remote';
			case 'remote':
				return 'Remote';
			default:
				return 'National';
		}
	}

	// Selected version + its items.
	let selectedVersionId = $state<number | null>(null);
	let items = $state<SupportItem[]>([]);
	let itemsLoading = $state(false);
	let itemsError = $state<string | null>(null);
	let itemSearch = $state('');

	const filteredItems = $derived.by<SupportItem[]>(() => {
		const q = itemSearch.trim().toLowerCase();
		if (q === '') return items;
		return items.filter(
			(it) => it.code.toLowerCase().includes(q) || it.name.toLowerCase().includes(q)
		);
	});

	// Expanded item prices (one at a time).
	let pricesItemId = $state<number | null>(null);
	let prices = $state<SupportItemPrice[]>([]);
	let pricesError = $state<string | null>(null);

	// Platform-admin upload form.
	let uploadLabel = $state('');
	let uploadEffectiveFrom = $state('');
	let uploadFile = $state<File | null>(null);
	let uploading = $state(false);
	let uploadError = $state<string | null>(null);
	let uploadNotice = $state<string | null>(null);

	onMount(() => {
		supportCatalog.ensureSubscribed();
		void (async () => {
			await supportCatalog.loadVersions();
			if (supportCatalog.versions.length > 0) {
				await selectVersion(supportCatalog.versions[0]);
			}
		})();
	});

	async function selectVersion(v: CatalogVersion): Promise<void> {
		selectedVersionId = v.id;
		pricesItemId = null;
		prices = [];
		itemsLoading = true;
		itemsError = null;
		try {
			items = await supportCatalog.loadItems(v.id);
		} catch (err) {
			itemsError = err instanceof Error ? err.message : 'Failed to load support items.';
			items = [];
		} finally {
			itemsLoading = false;
		}
	}

	async function togglePrices(item: SupportItem): Promise<void> {
		if (pricesItemId === item.id) {
			pricesItemId = null;
			prices = [];
			return;
		}
		pricesItemId = item.id;
		prices = [];
		pricesError = null;
		try {
			prices = await supportCatalog.loadPrices(item.id);
		} catch (err) {
			pricesError = err instanceof Error ? err.message : 'Failed to load prices.';
		}
	}

	function onFileChange(e: Event): void {
		const input = e.currentTarget as HTMLInputElement;
		uploadFile = input.files && input.files.length > 0 ? input.files[0] : null;
	}

	async function uploadVersion(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		uploadError = null;
		uploadNotice = null;
		if (uploadFile === null) {
			uploadError = 'Please choose an XLSX file.';
			return;
		}
		uploading = true;
		try {
			const form = new FormData();
			form.append('file', uploadFile);
			form.append('label', uploadLabel);
			form.append('effectiveFrom', uploadEffectiveFrom);
			await apiUpload<CatalogVersion>('/api/support-catalog/versions', form);
			uploadNotice = 'Catalogue version uploaded.';
			uploadLabel = '';
			uploadEffectiveFrom = '';
			uploadFile = null;
			await supportCatalog.loadVersions();
			if (supportCatalog.versions.length > 0) {
				await selectVersion(supportCatalog.versions[0]);
			}
		} catch (err) {
			uploadError = err instanceof Error ? err.message : 'Upload failed.';
		} finally {
			uploading = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<h1 class="mb-1 text-xl font-semibold">Support catalogue</h1>
		<p class="text-sm text-gray-500">
			The national NDIS Support Catalogue. Browse versions and their price caps by zone. This data is
			read-only.
		</p>
	</section>

	{#if session.isPlatformAdmin}
		<section class="rounded border border-amber-200 bg-amber-50 p-4">
			<h2 class="mb-1 text-base font-semibold">Upload a new catalogue version</h2>
			<p class="mb-4 text-sm text-gray-600">
				Platform-admin only. Upload the official NDIS Support Catalogue XLSX.
			</p>
			<form class="flex flex-wrap items-end gap-3" onsubmit={uploadVersion}>
				<label class="text-sm">
					<span class="mb-1 block font-medium">Label</span>
					<input
						type="text"
						bind:value={uploadLabel}
						required
						placeholder="2025-26 v1.1"
						class="w-48 rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="text-sm">
					<span class="mb-1 block font-medium">Effective from</span>
					<input
						type="date"
						bind:value={uploadEffectiveFrom}
						required
						class="rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="text-sm">
					<span class="mb-1 block font-medium">XLSX file</span>
					<input
						type="file"
						accept=".xlsx"
						onchange={onFileChange}
						class="text-sm"
					/>
				</label>
				<button
					type="submit"
					disabled={uploading}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{uploading ? 'Uploading…' : 'Upload'}
				</button>
			</form>
			{#if uploadError}
				<p class="mt-3 text-sm text-red-600">{uploadError}</p>
			{/if}
			{#if uploadNotice}
				<p class="mt-3 text-sm text-green-700">{uploadNotice}</p>
			{/if}
		</section>
	{/if}

	<section>
		<h2 class="mb-2 text-base font-semibold">Versions</h2>
		{#if supportCatalog.loading}
			<p class="text-sm text-gray-500">Loading versions…</p>
		{/if}
		{#if supportCatalog.error}
			<p class="text-sm text-red-600">{supportCatalog.error}</p>
		{/if}
		{#if supportCatalog.versions.length === 0 && !supportCatalog.loading}
			<p class="text-sm text-gray-500">No catalogue versions loaded yet.</p>
		{:else}
			<div class="flex flex-wrap gap-2">
				{#each supportCatalog.versions as v (v.id)}
					<button
						type="button"
						onclick={() => selectVersion(v)}
						class="rounded px-3 py-1 text-sm {selectedVersionId === v.id
							? 'bg-gray-900 text-white'
							: 'border border-gray-300 hover:bg-gray-50'}"
					>
						{v.label}
						<span class="opacity-70">({v.effectiveFrom ? v.effectiveFrom.slice(0, 10) : '—'})</span>
					</button>
				{/each}
			</div>
		{/if}
	</section>

	{#if selectedVersionId !== null}
		<section>
			<label class="mb-4 block max-w-sm">
				<span class="mb-1 block text-sm font-medium">Search support items</span>
				<input
					type="text"
					bind:value={itemSearch}
					placeholder="Filter by code or name"
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			{#if itemsLoading}
				<p class="text-sm text-gray-500">Loading items…</p>
			{/if}
			{#if itemsError}
				<p class="text-sm text-red-600">{itemsError}</p>
			{/if}

			<div class="overflow-hidden rounded border border-gray-200 bg-white">
				<table class="w-full text-sm">
					<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
						<tr>
							<th class="px-3 py-2 font-medium">Code</th>
							<th class="px-3 py-2 font-medium">Name</th>
							<th class="px-3 py-2 font-medium">Unit</th>
							<th class="px-3 py-2 font-medium">Category</th>
							<th class="px-3 py-2 font-medium">GST</th>
							<th class="px-3 py-2 font-medium text-right">Prices</th>
						</tr>
					</thead>
					<tbody>
						{#each filteredItems as item (item.id)}
							<tr class="border-b border-gray-100 last:border-0">
								<td class="px-3 py-2 font-mono text-xs">{item.code}</td>
								<td class="px-3 py-2 font-medium">{item.name}</td>
								<td class="px-3 py-2 text-gray-600">{item.unit || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{item.supportCategory || '—'}</td>
								<td class="px-3 py-2 text-gray-600">{item.gstFree ? 'GST-free' : 'Taxable'}</td>
								<td class="px-3 py-2 text-right">
									<button
										type="button"
										onclick={() => togglePrices(item)}
										class="text-gray-900 hover:underline"
									>
										{pricesItemId === item.id ? 'Hide' : 'Show caps'}
									</button>
								</td>
							</tr>
							{#if pricesItemId === item.id}
								<tr class="border-b border-gray-100 bg-gray-50">
									<td colspan="6" class="px-3 py-3">
										{#if pricesError}
											<p class="text-sm text-red-600">{pricesError}</p>
										{:else if prices.length === 0}
											<p class="text-sm text-gray-500">No published prices.</p>
										{:else}
											<div class="flex flex-wrap gap-4 text-sm">
												{#each prices as price (price.id)}
													<span class="rounded border border-gray-200 bg-white px-3 py-1">
														{zoneLabel(price.zone)}:
														<span class="font-medium">{money(price.priceCap)}</span>
													</span>
												{/each}
											</div>
										{/if}
									</td>
								</tr>
							{/if}
						{:else}
							<tr>
								<td colspan="6" class="px-3 py-6 text-center text-gray-500">
									No support items found.
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}
</div>
