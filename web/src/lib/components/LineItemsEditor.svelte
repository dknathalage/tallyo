<script lang="ts">
	import { supportCatalog } from '$lib/stores/supportCatalog.svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import type { SupportItem, SupportItemPrice, ValidationDetail, Zone } from '$lib/api/types';

	// An editor row. `kind` distinguishes an NDIS support-item line (code-driven,
	// gst server-authoritative) from a custom line (free text, user gst).
	export interface EditorLine {
		kind: 'support' | 'custom';
		customItemId: string | null;
		// Pinned NDIS catalogue version uuid for an EXISTING support line; null for a
		// new line (the server then prices it from the current version). Carried on
		// edit so re-validation never re-prices an existing line against a newer one.
		catalogVersionId: string | null;
		code: string;
		description: string;
		serviceDate: string;
		unit: string;
		quantity: number;
		unitPrice: number;
		gstFree: boolean;
		sortOrder: number;
	}

	interface Props {
		lines: EditorLine[];
		// Per-line validation details keyed by line index (from a 422 response).
		details?: ValidationDetail[];
	}

	let { lines = $bindable(), details = [] }: Props = $props();

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	const zone = $derived<Zone>((businessProfile.profile.zone as Zone) || 'national');

	// Latest catalogue version's support items (for the code picker).
	let catalogItems = $state<SupportItem[]>([]);
	let catalogLoaded = $state(false);

	async function ensureCatalog(): Promise<void> {
		if (catalogLoaded) return;
		catalogLoaded = true;
		await supportCatalog.loadVersions();
		if (supportCatalog.versions.length > 0) {
			try {
				catalogItems = await supportCatalog.loadItems(supportCatalog.versions[0].id);
			} catch {
				catalogItems = [];
			}
		}
	}

	// Price-cap cache keyed by supportItem id → the cap for the tenant zone.
	let capCache = $state<Record<number, number | null>>({});

	async function capFor(item: SupportItem): Promise<number | null> {
		if (item.id in capCache) return capCache[item.id];
		try {
			const prices: SupportItemPrice[] = await supportCatalog.loadPrices(item.id);
			const match = prices.find((p) => p.zone === zone);
			const cap = match ? match.priceCap : null;
			capCache = { ...capCache, [item.id]: cap };
			return cap;
		} catch {
			return null;
		}
	}

	// Per-row picker UI state.
	let pickerOpen = $state<number | null>(null);
	let pickerSearch = $state('');
	// The applicable price cap shown next to a row after selecting an item.
	let rowCap = $state<Record<number, number | null>>({});

	const pickerResults = $derived.by<SupportItem[]>(() => {
		const q = pickerSearch.trim().toLowerCase();
		if (q === '') return catalogItems.slice(0, 20);
		return catalogItems
			.filter((it) => it.code.toLowerCase().includes(q) || it.name.toLowerCase().includes(q))
			.slice(0, 20);
	});

	function lineAmount(row: EditorLine): number {
		return (Number(row.quantity) || 0) * (Number(row.unitPrice) || 0);
	}

	function detailsFor(index: number): ValidationDetail[] {
		return details.filter((d) => d.line === index);
	}

	function fieldError(index: number, field: string): string | null {
		const d = details.find((x) => x.line === index && x.field === field);
		return d ? d.message : null;
	}

	export function addSupportLine(): void {
		lines.push({
			kind: 'support',
			customItemId: null,
			catalogVersionId: null,
			code: '',
			description: '',
			serviceDate: new Date().toISOString().slice(0, 10),
			unit: '',
			quantity: 1,
			unitPrice: 0,
			gstFree: true,
			sortOrder: lines.length
		});
		void ensureCatalog();
	}

	export function addCustomLine(): void {
		lines.push({
			kind: 'custom',
			customItemId: null,
			catalogVersionId: null,
			code: '',
			description: '',
			serviceDate: '',
			unit: '',
			quantity: 1,
			unitPrice: 0,
			gstFree: true,
			sortOrder: lines.length
		});
	}

	function removeLine(index: number): void {
		lines.splice(index, 1);
	}

	async function openPicker(index: number): Promise<void> {
		pickerOpen = pickerOpen === index ? null : index;
		pickerSearch = '';
		if (pickerOpen !== null) await ensureCatalog();
	}

	async function pickItem(index: number, item: SupportItem): Promise<void> {
		const row = lines[index];
		row.code = item.code;
		row.description = item.name;
		row.unit = item.unit;
		row.gstFree = item.gstFree; // server-authoritative; shown read-only.
		pickerOpen = null;
		const cap = await capFor(item);
		rowCap = { ...rowCap, [index]: cap };
	}

	function selectCustomItem(index: number, e: Event): void {
		const id = (e.currentTarget as HTMLSelectElement).value;
		const row = lines[index];
		if (id === '') {
			row.customItemId = null;
			return;
		}
		const ci = customItems.items.find((c) => String(c.id) === id);
		if (ci) {
			row.customItemId = ci.id;
			row.description = ci.name;
			row.unit = ci.unit;
			row.unitPrice = ci.rate;
			row.gstFree = ci.gstFree;
		}
	}
</script>

<div class="space-y-3">
	<div class="flex items-center justify-between">
		<span class="text-sm font-medium">Line items</span>
		<div class="flex gap-2">
			<button
				type="button"
				onclick={addSupportLine}
				class="rounded border border-gray-300 px-3 py-1 text-sm hover:bg-gray-50"
			>
				Add NDIS line
			</button>
			<button
				type="button"
				onclick={addCustomLine}
				class="rounded border border-gray-300 px-3 py-1 text-sm hover:bg-gray-50"
			>
				Add custom line
			</button>
		</div>
	</div>

	{#each lines as line, i (i)}
		<div class="rounded border border-gray-200 p-3">
			<div class="mb-2 flex items-center justify-between">
				<span class="text-xs font-semibold tracking-wide text-gray-500 uppercase">
					{line.kind === 'support' ? 'NDIS support item' : 'Custom item'}
				</span>
				<button
					type="button"
					onclick={() => removeLine(i)}
					class="text-sm text-red-600 hover:underline"
					aria-label="Remove line"
				>
					Remove
				</button>
			</div>

			{#if line.kind === 'support'}
				<div class="grid grid-cols-12 gap-2">
					<div class="col-span-12 sm:col-span-3">
						<span class="mb-1 block text-xs font-medium text-gray-500">Code</span>
						<div class="flex gap-1">
							<input
								type="text"
								bind:value={line.code}
								placeholder="e.g. 01_011_0107_1_1"
								class="w-full rounded border border-gray-300 px-2 py-1 font-mono text-xs"
							/>
							<button
								type="button"
								onclick={() => openPicker(i)}
								class="shrink-0 rounded border border-gray-300 px-2 text-xs hover:bg-gray-50"
							>
								Find
							</button>
						</div>
						{#if fieldError(i, 'code')}
							<p class="mt-1 text-xs text-red-600">{fieldError(i, 'code')}</p>
						{/if}
					</div>
					<div class="col-span-12 sm:col-span-3">
						<span class="mb-1 block text-xs font-medium text-gray-500">Service date</span>
						<input
							type="date"
							bind:value={line.serviceDate}
							class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
						/>
						{#if fieldError(i, 'serviceDate')}
							<p class="mt-1 text-xs text-red-600">{fieldError(i, 'serviceDate')}</p>
						{/if}
					</div>
					<div class="col-span-12 sm:col-span-6">
						<span class="mb-1 block text-xs font-medium text-gray-500">Description</span>
						<input
							type="text"
							bind:value={line.description}
							class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
						/>
					</div>
				</div>

				{#if pickerOpen === i}
					<div class="mt-2 rounded border border-gray-200 bg-gray-50 p-2">
						<input
							type="text"
							bind:value={pickerSearch}
							placeholder="Search support items by code or name"
							class="mb-2 w-full rounded border border-gray-300 px-2 py-1 text-sm"
						/>
						{#if !catalogLoaded}
							<p class="text-xs text-gray-500">Loading catalogue…</p>
						{:else if catalogItems.length === 0}
							<p class="text-xs text-gray-500">No catalogue loaded.</p>
						{:else}
							<ul class="max-h-48 overflow-auto text-sm">
								{#each pickerResults as it (it.id)}
									<li>
										<button
											type="button"
											onclick={() => pickItem(i, it)}
											class="flex w-full items-center justify-between rounded px-2 py-1 text-left hover:bg-white"
										>
											<span><span class="font-mono text-xs">{it.code}</span> — {it.name}</span>
											<span class="text-xs text-gray-500">{it.gstFree ? 'GST-free' : 'Taxable'}</span>
										</button>
									</li>
								{:else}
									<li class="px-2 py-1 text-xs text-gray-500">No matches.</li>
								{/each}
							</ul>
						{/if}
					</div>
				{/if}
			{:else}
				<div class="grid grid-cols-12 gap-2">
					<div class="col-span-12 sm:col-span-4">
						<span class="mb-1 block text-xs font-medium text-gray-500">From custom item</span>
						<select
							onchange={(e) => selectCustomItem(i, e)}
							class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
						>
							<option value="">— manual —</option>
							{#each customItems.items as ci (ci.id)}
								<option value={String(ci.id)}>{ci.name}</option>
							{/each}
						</select>
					</div>
					<div class="col-span-12 sm:col-span-8">
						<span class="mb-1 block text-xs font-medium text-gray-500">Description</span>
						<input
							type="text"
							bind:value={line.description}
							class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
						/>
					</div>
				</div>
			{/if}

			<div class="mt-2 grid grid-cols-12 items-end gap-2">
				<div class="col-span-6 sm:col-span-2">
					<span class="mb-1 block text-xs font-medium text-gray-500">Unit</span>
					<input
						type="text"
						bind:value={line.unit}
						class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
					/>
				</div>
				<div class="col-span-6 sm:col-span-2">
					<span class="mb-1 block text-xs font-medium text-gray-500">Qty</span>
					<input
						type="number"
						step="any"
						bind:value={line.quantity}
						class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
					/>
					{#if fieldError(i, 'quantity')}
						<p class="mt-1 text-xs text-red-600">{fieldError(i, 'quantity')}</p>
					{/if}
				</div>
				<div class="col-span-6 sm:col-span-3">
					<span class="mb-1 block text-xs font-medium text-gray-500">Unit price</span>
					<input
						type="number"
						step="any"
						bind:value={line.unitPrice}
						class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
					/>
					{#if fieldError(i, 'unitPrice')}
						<p class="mt-1 text-xs text-red-600">{fieldError(i, 'unitPrice')}</p>
					{/if}
					{#if line.kind === 'support' && i in rowCap}
						<p class="mt-1 text-xs text-gray-500">
							Cap ({zone}): {rowCap[i] === null ? 'Quote' : money(rowCap[i] as number)}
						</p>
					{/if}
				</div>
				<div class="col-span-6 sm:col-span-2">
					<span class="mb-1 block text-xs font-medium text-gray-500">GST</span>
					{#if line.kind === 'support'}
						<p class="px-1 py-1 text-sm text-gray-700">
							{line.gstFree ? 'GST-free' : 'Taxable'}
						</p>
					{:else}
						<label class="flex items-center gap-1 py-1">
							<input type="checkbox" bind:checked={line.gstFree} class="h-4 w-4" />
							<span class="text-xs">GST-free</span>
						</label>
					{/if}
				</div>
				<div class="col-span-12 text-right sm:col-span-3">
					<span class="mb-1 block text-xs font-medium text-gray-500">Amount</span>
					<span class="text-sm">{money(lineAmount(line))}</span>
				</div>
			</div>

			{#each detailsFor(i).filter((d) => !['code', 'serviceDate', 'quantity', 'unitPrice'].includes(d.field)) as d (d.field + d.message)}
				<p class="mt-1 text-xs text-red-600">{d.message}</p>
			{/each}
		</div>
	{:else}
		<p class="text-sm text-gray-500">No line items.</p>
	{/each}
</div>
