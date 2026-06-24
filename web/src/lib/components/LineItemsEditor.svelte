<script lang="ts">
	import { priceList } from '$lib/stores/priceList.svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import Button from '$lib/components/Button.svelte';
	import type { Item, LineItem, ValidationDetail } from '$lib/api/types';

	// An editor row. `kind` distinguishes a price-list item line (code-driven,
	// gst server-authoritative) from a custom line (free text, user gst).
	export interface EditorLine {
		kind: 'support' | 'custom';
		customItemId: string | null;
		// Pinned price-list version uuid for an EXISTING support line; null for a
		// new line (the server then prices it from the current version). Carried on
		// edit so re-validation never re-prices an existing line against a newer one.
		priceListVersionId: string | null;
		code: string;
		description: string;
		serviceDate: string;
		unit: string;
		quantity: number;
		unitPrice: number;
		taxable: boolean;
		sortOrder: number;
		// True when this line was proposed by an AI Smart (shows a subtle marker so
		// the user knows to review it). Always editable; never auto-submitted.
		aiSuggested?: boolean;
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

	// Latest price-list version's items (for the code picker).
	let catalogItems = $state<Item[]>([]);
	let catalogLoaded = $state(false);

	async function ensureCatalog(): Promise<void> {
		if (catalogLoaded) return;
		catalogLoaded = true;
		await priceList.loadVersions();
		if (priceList.versions.length > 0) {
			try {
				catalogItems = await priceList.loadItems(priceList.versions[0].id);
			} catch {
				catalogItems = [];
			}
		}
	}

	// Per-row picker UI state.
	let pickerOpen = $state<number | null>(null);
	let pickerSearch = $state('');

	const pickerResults = $derived.by<Item[]>(() => {
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
			priceListVersionId: null,
			code: '',
			description: '',
			serviceDate: new Date().toISOString().slice(0, 10),
			unit: '',
			quantity: 1,
			unitPrice: 0,
			taxable: false,
			sortOrder: lines.length
		});
		void ensureCatalog();
	}

	export function addCustomLine(): void {
		lines.push({
			kind: 'custom',
			customItemId: null,
			priceListVersionId: null,
			code: '',
			description: '',
			serviceDate: '',
			unit: '',
			quantity: 1,
			unitPrice: 0,
			taxable: false,
			sortOrder: lines.length
		});
	}

	// Append AI-suggested catalogue-priced lines (from a LineItem[] payload) as
	// editable rows, flagged aiSuggested so the user reviews them before saving.
	export function addLines(suggested: LineItem[]): void {
		for (let i = 0; i < suggested.length; i++) {
			const s = suggested[i];
			lines.push({
				kind: s.itemId !== null || s.code !== '' ? 'support' : 'custom',
				customItemId: s.customItemId,
				priceListVersionId: s.priceListVersionId,
				code: s.code,
				description: s.description,
				serviceDate: s.serviceDate,
				unit: s.unit,
				quantity: s.quantity,
				unitPrice: s.unitPrice,
				taxable: s.taxable,
				sortOrder: lines.length,
				aiSuggested: true
			});
		}
	}

	function removeLine(index: number): void {
		lines.splice(index, 1);
	}

	async function openPicker(index: number): Promise<void> {
		pickerOpen = pickerOpen === index ? null : index;
		pickerSearch = '';
		if (pickerOpen !== null) await ensureCatalog();
	}

	function pickItem(index: number, item: Item): void {
		const row = lines[index];
		row.code = item.code;
		row.description = item.name;
		row.unit = item.unit;
		row.taxable = item.taxable; // server-authoritative; shown read-only.
		// Pre-fill the unit price from the catalogue item's generic price when the
		// row has none yet (the server fills the same way on submit).
		if ((Number(row.unitPrice) || 0) <= 0 && item.unitPrice != null) {
			row.unitPrice = item.unitPrice;
		}
		pickerOpen = null;
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
			row.taxable = ci.taxable;
		}
	}
</script>

<div class="space-y-3">
	<div class="flex items-center justify-between">
		<span class="text-sm font-medium">Line items</span>
		<div class="flex gap-2">
			<Button variant="secondary" size="sm" onclick={addSupportLine}>
				Add catalogue line
			</Button>
			<Button variant="secondary" size="sm" onclick={addCustomLine}>
				Add custom line
			</Button>
		</div>
	</div>

	{#each lines as line, i (i)}
		<div class="rounded-lg border border-gray-200 p-3">
			<div class="mb-2 flex items-center justify-between">
				<span class="flex items-center gap-2 text-xs font-semibold tracking-wide text-gray-500 uppercase">
					{line.kind === 'support' ? 'Catalogue item' : 'Custom item'}
					{#if line.aiSuggested}
						<span class="rounded-full bg-brand-50 px-2 py-0.5 text-[10px] font-medium text-brand-700 normal-case tracking-normal">
							✨ AI suggested · review
						</span>
					{/if}
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
								class="w-full rounded-lg border border-gray-300 px-2 py-1 font-mono tabular-nums text-xs"
							/>
							<button
								type="button"
								onclick={() => openPicker(i)}
								class="shrink-0 rounded-lg border border-gray-300 px-2 text-xs hover:bg-gray-50"
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
							class="w-full rounded-lg border border-gray-300 px-2 py-1 text-sm"
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
							class="w-full rounded-lg border border-gray-300 px-2 py-1 text-sm"
						/>
					</div>
				</div>

				{#if pickerOpen === i}
					<div class="mt-2 rounded-lg border border-gray-200 bg-gray-50 p-2">
						<input
							type="text"
							bind:value={pickerSearch}
							placeholder="Search support items by code or name"
							class="mb-2 w-full rounded-lg border border-gray-300 px-2 py-1 text-sm"
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
											class="flex w-full items-center justify-between rounded-lg px-2 py-1 text-left hover:bg-white"
										>
											<span><span class="font-mono tabular-nums text-xs">{it.code}</span> — {it.name}</span>
											<span class="text-xs text-gray-500">{it.taxable ? 'Taxable' : 'GST-free'}</span>
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
							class="w-full rounded-lg border border-gray-300 px-2 py-1 text-sm"
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
							class="w-full rounded-lg border border-gray-300 px-2 py-1 text-sm"
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
						class="w-full rounded-lg border border-gray-300 px-2 py-1 text-sm"
					/>
				</div>
				<div class="col-span-6 sm:col-span-2">
					<span class="mb-1 block text-xs font-medium text-gray-500">Qty</span>
					<input
						type="number"
						step="any"
						bind:value={line.quantity}
						class="w-full rounded-lg border border-gray-300 px-2 py-1 text-sm font-mono tabular-nums"
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
						class="w-full rounded-lg border border-gray-300 px-2 py-1 text-sm font-mono tabular-nums"
					/>
					{#if fieldError(i, 'unitPrice')}
						<p class="mt-1 text-xs text-red-600">{fieldError(i, 'unitPrice')}</p>
					{/if}
				</div>
				<div class="col-span-6 sm:col-span-2">
					<span class="mb-1 block text-xs font-medium text-gray-500">GST</span>
					{#if line.kind === 'support'}
						<p class="px-1 py-1 text-sm text-gray-700">
							{line.taxable ? 'Taxable' : 'GST-free'}
						</p>
					{:else}
						<label class="flex items-center gap-1 py-1">
							<input type="checkbox" bind:checked={line.taxable} class="h-4 w-4" />
							<span class="text-xs">Taxable</span>
						</label>
					{/if}
				</div>
				<div class="col-span-12 text-right sm:col-span-3">
					<span class="mb-1 block text-xs font-medium text-gray-500">Amount</span>
					<span class="text-sm font-mono tabular-nums">{money(lineAmount(line))}</span>
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
