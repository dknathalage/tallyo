<script lang="ts">
	import { onMount } from 'svelte';
	import { priceList } from '$lib/stores/priceList.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { apiUpload, tenantPath } from '$lib/api/client';
	import type { PriceListVersion, Item, ItemPrice } from '$lib/api/types';

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
	let selectedVersionId = $state<string | null>(null);
	let items = $state<Item[]>([]);
	let itemsLoading = $state(false);
	let itemsError = $state<string | null>(null);
	let itemSearch = $state('');

	const filteredItems = $derived.by<Item[]>(() => {
		const q = itemSearch.trim().toLowerCase();
		if (q === '') return items;
		return items.filter(
			(it) => it.code.toLowerCase().includes(q) || it.name.toLowerCase().includes(q)
		);
	});

	// Expanded item prices (one at a time).
	let pricesItemId = $state<string | null>(null);
	let prices = $state<ItemPrice[]>([]);
	let pricesError = $state<string | null>(null);

	// Owner/admin two-step import wizard.
	const TARGETS = ['name', 'code', 'unit', 'category', 'unitPrice', 'taxable'] as const;
	let importLabel = $state('');
	let importHeaderRow = $state(1);
	let importFile = $state<File | null>(null);
	let importing = $state(false);
	let inspecting = $state(false);
	let importError = $state<string | null>(null);
	let importNotice = $state<string | null>(null);
	// Inspect result + the per-header mapping selections ('' = ignore).
	let inspectHeaders = $state<string[]>([]);
	let inspectSample = $state<Record<string, string>[]>([]);
	let mapping = $state<Record<string, string>>({});

	type InspectResult = { headers: string[]; sampleRows: Record<string, string>[] };

	// The preview maps each sample row through the current mapping into the target
	// fields, so the user sees what will be imported before committing.
	const mappedPreview = $derived.by<Record<string, string>[]>(() => {
		return inspectSample.map((row) => {
			const out: Record<string, string> = {};
			for (const header of inspectHeaders) {
				const target = mapping[header];
				if (target && target !== '') out[target] = row[header] ?? '';
			}
			return out;
		});
	});

	const hasNameMapped = $derived(Object.values(mapping).includes('name'));

	onMount(() => {
		priceList.ensureSubscribed();
		void (async () => {
			await priceList.loadVersions();
			if (priceList.versions.length > 0) {
				await selectVersion(priceList.versions[0]);
			}
		})();
	});

	async function selectVersion(v: PriceListVersion): Promise<void> {
		selectedVersionId = v.id;
		pricesItemId = null;
		prices = [];
		itemsLoading = true;
		itemsError = null;
		try {
			items = await priceList.loadItems(v.id);
		} catch (err) {
			itemsError = err instanceof Error ? err.message : 'Failed to load items.';
			items = [];
		} finally {
			itemsLoading = false;
		}
	}

	async function togglePrices(item: Item): Promise<void> {
		if (pricesItemId === item.id) {
			pricesItemId = null;
			prices = [];
			return;
		}
		pricesItemId = item.id;
		prices = [];
		pricesError = null;
		try {
			prices = await priceList.loadPrices(item.id);
		} catch (err) {
			pricesError = err instanceof Error ? err.message : 'Failed to load prices.';
		}
	}

	function onFileChange(e: Event): void {
		const input = e.currentTarget as HTMLInputElement;
		importFile = input.files && input.files.length > 0 ? input.files[0] : null;
		// A new file invalidates any prior inspection.
		inspectHeaders = [];
		inspectSample = [];
		mapping = {};
		importNotice = null;
	}

	async function inspectFile(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		importError = null;
		importNotice = null;
		if (importFile === null) {
			importError = 'Please choose a CSV or XLSX file.';
			return;
		}
		inspecting = true;
		try {
			const form = new FormData();
			form.append('file', importFile);
			form.append('headerRow', String(importHeaderRow));
			const res = await apiUpload<InspectResult>(tenantPath('price-list/import/inspect'), form);
			inspectHeaders = res?.headers ?? [];
			inspectSample = res?.sampleRows ?? [];
			// Default mapping: ignore everything until the user chooses.
			const next: Record<string, string> = {};
			for (const h of inspectHeaders) next[h] = '';
			mapping = next;
		} catch (err) {
			importError = err instanceof Error ? err.message : 'Inspect failed.';
			inspectHeaders = [];
			inspectSample = [];
		} finally {
			inspecting = false;
		}
	}

	async function commitImport(): Promise<void> {
		importError = null;
		importNotice = null;
		if (importFile === null) {
			importError = 'Please choose a file and inspect it first.';
			return;
		}
		if (importLabel.trim() === '') {
			importError = 'Please enter a label.';
			return;
		}
		if (!hasNameMapped) {
			importError = 'Map one column to "name" (required).';
			return;
		}
		// Drop ignored columns from the mapping sent to the server.
		const cleanMapping: Record<string, string> = {};
		for (const [header, target] of Object.entries(mapping)) {
			if (target && target !== '') cleanMapping[header] = target;
		}
		importing = true;
		try {
			const form = new FormData();
			form.append('file', importFile);
			form.append('label', importLabel);
			form.append('headerRow', String(importHeaderRow));
			form.append('mapping', JSON.stringify(cleanMapping));
			await apiUpload<PriceListVersion>(tenantPath('price-list/import/commit'), form);
			importNotice = 'Price-list version imported.';
			importLabel = '';
			importFile = null;
			inspectHeaders = [];
			inspectSample = [];
			mapping = {};
			await priceList.loadVersions();
			if (priceList.versions.length > 0) {
				await selectVersion(priceList.versions[0]);
			}
		} catch (err) {
			importError = err instanceof Error ? err.message : 'Import failed.';
		} finally {
			importing = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<h1 class="mb-1 text-xl font-semibold">Price list</h1>
		<p class="text-sm text-gray-500">
			Your tenant's price list. Browse versions and their price caps by zone. This data is
			read-only.
		</p>
	</section>

	{#if session.isManager}
		<section class="rounded border border-amber-200 bg-amber-50 p-4">
			<h2 class="mb-1 text-base font-semibold">Import a new price-list version</h2>
			<p class="mb-4 text-sm text-gray-600">
				Owner/admin only. Upload a CSV or XLSX, map its columns to the price-list fields, then
				import.
			</p>

			<form class="flex flex-wrap items-end gap-3" onsubmit={inspectFile}>
				<label class="text-sm">
					<span class="mb-1 block font-medium">File</span>
					<input type="file" accept=".csv,.xlsx" onchange={onFileChange} class="text-sm" />
				</label>
				<label class="text-sm">
					<span class="mb-1 block font-medium">Header row</span>
					<input
						type="number"
						min="1"
						bind:value={importHeaderRow}
						class="w-20 rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<button
					type="submit"
					disabled={inspecting || importFile === null}
					class="rounded border border-gray-300 px-4 py-2 text-sm font-medium hover:bg-gray-50 disabled:opacity-50"
				>
					{inspecting ? 'Inspecting…' : 'Inspect columns'}
				</button>
			</form>

			{#if inspectHeaders.length > 0}
				<div class="mt-4 space-y-4">
					<div>
						<h3 class="mb-2 text-sm font-semibold">Map columns</h3>
						<div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
							{#each inspectHeaders as header (header)}
								<label class="text-sm">
									<span class="mb-1 block font-medium">{header}</span>
									<select
										bind:value={mapping[header]}
										class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
									>
										<option value="">— ignore —</option>
										{#each TARGETS as target (target)}
											<option value={target}>{target}</option>
										{/each}
									</select>
								</label>
							{/each}
						</div>
						{#if !hasNameMapped}
							<p class="mt-2 text-sm text-amber-700">Map one column to <code>name</code> (required).</p>
						{/if}
					</div>

					{#if mappedPreview.length > 0}
						<div>
							<h3 class="mb-2 text-sm font-semibold">Preview</h3>
							<div class="overflow-x-auto rounded border border-gray-200 bg-white">
								<table class="w-full text-sm">
									<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
										<tr>
											{#each TARGETS as target (target)}
												<th class="px-3 py-2 font-medium">{target}</th>
											{/each}
										</tr>
									</thead>
									<tbody>
										{#each mappedPreview as row, i (i)}
											<tr class="border-b border-gray-100 last:border-0">
												{#each TARGETS as target (target)}
													<td class="px-3 py-2 text-gray-700">{row[target] ?? '—'}</td>
												{/each}
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						</div>
					{/if}

					<div class="flex flex-wrap items-end gap-3">
						<label class="text-sm">
							<span class="mb-1 block font-medium">Label</span>
							<input
								type="text"
								bind:value={importLabel}
								placeholder="2025-26 v1.1"
								class="w-48 rounded border border-gray-300 px-3 py-2 text-sm"
							/>
						</label>
						<button
							type="button"
							onclick={commitImport}
							disabled={importing || !hasNameMapped || importLabel.trim() === ''}
							class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
						>
							{importing ? 'Importing…' : 'Import'}
						</button>
					</div>
				</div>
			{/if}

			{#if importError}
				<p class="mt-3 text-sm text-red-600">{importError}</p>
			{/if}
			{#if importNotice}
				<p class="mt-3 text-sm text-green-700">{importNotice}</p>
			{/if}
		</section>
	{/if}

	<section>
		<h2 class="mb-2 text-base font-semibold">Versions</h2>
		{#if priceList.loading}
			<p class="text-sm text-gray-500">Loading versions…</p>
		{/if}
		{#if priceList.error}
			<p class="text-sm text-red-600">{priceList.error}</p>
		{/if}
		{#if priceList.versions.length === 0 && !priceList.loading}
			<p class="text-sm text-gray-500">No price-list versions loaded yet.</p>
		{:else}
			<div class="flex flex-wrap gap-2">
				{#each priceList.versions as v (v.id)}
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
				<span class="mb-1 block text-sm font-medium">Search items</span>
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
							<th class="px-3 py-2 font-medium text-right">Unit price</th>
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
								<td class="px-3 py-2 text-gray-600">{item.category || '—'}</td>
								<td class="px-3 py-2 text-right text-gray-600"
									>{item.unitPrice === null ? '—' : money(item.unitPrice)}</td
								>
								<td class="px-3 py-2 text-gray-600">{item.taxable ? 'Taxable' : 'GST-free'}</td>
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
									<td colspan="7" class="px-3 py-3">
										{#if pricesError}
											<p class="text-sm text-red-600">{pricesError}</p>
										{:else if prices.length === 0}
											<p class="text-sm text-gray-500">No published prices.</p>
										{:else}
											<div class="flex flex-wrap gap-4 text-sm">
												{#each prices as price (price.zone)}
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
								<td colspan="7" class="px-3 py-6 text-center text-gray-500">
									No items found.
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}
</div>
