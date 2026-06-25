<script lang="ts">
	import { onMount } from 'svelte';
	import { catalogue } from '$lib/stores/catalogue.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { apiUpload, tenantPath } from '$lib/api/client';
	import { features } from '$lib/stores/features.svelte';
	import * as smarts from '$lib/api/smarts';
	import { t } from '$lib/nav';
	import DataTable from '$lib/components/DataTable.svelte';
	import CreateModal from '$lib/components/CreateModal.svelte';
	import Button from '$lib/components/Button.svelte';
	import Sparkle from '$lib/components/Sparkle.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type { CatalogueItem, CatalogueItemInput } from '$lib/api/types';

	onMount(() => {
		catalogue.ensureSubscribed();
		void catalogue.query({ page: 1, limit: 50 });
	});

	let createOpen = $state(false);

	function toInput(c: CatalogueItem): CatalogueItemInput {
		return {
			code: c.code,
			name: c.name,
			unit: c.unit,
			category: c.category,
			unitPrice: Number(c.unitPrice),
			taxable: c.taxable,
			metadata: c.metadata ?? ''
		};
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	// DataTable columns. Keys match CatalogueItem JSON fields (and the server
	// allowlist), so one key drives filter, sort, display, and edit-page input kind.
	const columns: Column<CatalogueItem>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'code', label: 'Code', sortable: false, filter: 'text' },
		{
			key: 'unitPrice',
			label: 'Unit price',
			sortable: true,
			filter: 'number',
			input: 'number',
			cell: (c) => c.unitPrice.toFixed(2)
		},
		{ key: 'unit', label: 'Unit', sortable: true, filter: 'text' },
		{ key: 'category', label: 'Category', sortable: true, filter: 'text' },
		{
			key: 'taxable',
			label: 'GST',
			sortable: true,
			input: 'checkbox',
			cell: (c) => (c.taxable ? 'Taxable' : '—')
		}
	];

	const rowActions: RowAction<CatalogueItem>[] = [
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await catalogue.crud.remove(r.id); // bounded by selection
			}
		}
	];

	// ── Owner/admin upload-and-map import (upserts the catalogue by code). ──
	const TARGETS = ['name', 'code', 'unit', 'category', 'unitPrice', 'taxable'] as const;
	let importHeaderRow = $state(1);
	let importFile = $state<File | null>(null);
	let importing = $state(false);
	let inspecting = $state(false);
	let importError = $state<string | null>(null);
	let importNotice = $state<string | null>(null);
	let inspectHeaders = $state<string[]>([]);
	let inspectSample = $state<Record<string, string>[]>([]);
	let mapping = $state<Record<string, string>>({});

	type InspectResult = { headers: string[]; sampleRows: Record<string, string>[] };
	type ImportSummary = { created: number; updated: number };

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

	let autoMapping = $state(false);
	let autoMapError = $state<string | null>(null);
	const VALID_TARGETS = new Set<string>(TARGETS);

	async function autoMap(): Promise<void> {
		autoMapError = null;
		if (inspectHeaders.length === 0) return;
		autoMapping = true;
		try {
			const proposed = await smarts.mapImport(inspectHeaders, inspectSample);
			const next = { ...mapping };
			for (const header of inspectHeaders) {
				const target = proposed[header];
				if (target && VALID_TARGETS.has(target)) next[header] = target;
			}
			mapping = next;
		} catch (err) {
			autoMapError = err instanceof Error ? err.message : 'Auto-map failed.';
		} finally {
			autoMapping = false;
		}
	}

	function onFileChange(e: Event): void {
		const input = e.currentTarget as HTMLInputElement;
		importFile = input.files && input.files.length > 0 ? input.files[0] : null;
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
			const res = await apiUpload<InspectResult>(tenantPath('catalogue/import/inspect'), form);
			inspectHeaders = res?.headers ?? [];
			inspectSample = res?.sampleRows ?? [];
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
		if (!hasNameMapped) {
			importError = 'Map one column to "name" (required).';
			return;
		}
		const cleanMapping: Record<string, string> = {};
		for (const [header, target] of Object.entries(mapping)) {
			if (target && target !== '') cleanMapping[header] = target;
		}
		importing = true;
		try {
			const form = new FormData();
			form.append('file', importFile);
			form.append('headerRow', String(importHeaderRow));
			form.append('mapping', JSON.stringify(cleanMapping));
			const res = await apiUpload<ImportSummary>(tenantPath('catalogue/import/commit'), form);
			importNotice = `Imported: ${res?.created ?? 0} created, ${res?.updated ?? 0} updated.`;
			importFile = null;
			inspectHeaders = [];
			inspectSample = [];
			mapping = {};
			await catalogue.query({ page: 1, limit: 50 });
		} catch (err) {
			importError = err instanceof Error ? err.message : 'Import failed.';
		} finally {
			importing = false;
		}
	}
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-2xl font-semibold tracking-tight">Catalogue</h1>
			<p class="text-sm text-gray-500">
				Your tenant's reusable priced line items. Add them to invoices and estimates; editing a
				priced item that an existing document already uses keeps that document's price.
			</p>
		</div>
	</section>

	{#if session.isManager}
		<section class="rounded-xl border border-amber-200 bg-amber-50 p-4 shadow-sm">
			<h2 class="mb-1 text-base font-semibold">Import items</h2>
			<p class="mb-4 text-sm text-gray-600">
				Owner/admin only. Upload a CSV or XLSX, map its columns to the catalogue fields, then
				import. Existing items are matched and updated by code.
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
						class="w-20 rounded-lg border border-gray-300 px-3 py-2 text-sm tabular-nums"
					/>
				</label>
				<Button
					type="submit"
					variant="secondary"
					loading={inspecting}
					disabled={inspecting || importFile === null}
				>
					{inspecting ? 'Inspecting…' : 'Inspect columns'}
				</Button>
			</form>

			{#if inspectHeaders.length > 0}
				<div class="mt-4 space-y-4">
					<div>
						<div class="mb-2 flex items-center justify-between gap-3">
							<h3 class="text-sm font-semibold">Map columns</h3>
							{#if features.smarts}
								<Button
									type="button"
									variant="secondary"
									size="sm"
									loading={autoMapping}
									disabled={autoMapping}
									onclick={autoMap}
								>
									<Sparkle /> Auto-map columns
								</Button>
							{/if}
						</div>
						{#if autoMapError}<p class="mb-2 text-sm text-red-600">{autoMapError}</p>{/if}
						<div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
							{#each inspectHeaders as header (header)}
								<label class="text-sm">
									<span class="mb-1 block font-medium">{header}</span>
									<select
										bind:value={mapping[header]}
										class="w-full rounded-lg border border-gray-300 px-2 py-1.5 text-sm"
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
							<div class="overflow-x-auto rounded-lg border border-gray-200 bg-white">
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
						<Button onclick={commitImport} loading={importing} disabled={importing || !hasNameMapped}>
							{importing ? 'Importing…' : 'Import'}
						</Button>
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
		{#if catalogue.error}
			<p class="mb-3 text-sm text-red-600">{catalogue.error}</p>
		{/if}

		<DataTable
			title="Catalogue"
			{columns}
			store={catalogue}
			{rowActions}
			rowHref={(r) => t(`/catalogue/${r.id}`)}
			onnew={() => (createOpen = true)}
		/>
	</section>
</div>

<CreateModal
	title="catalogue item"
	{columns}
	create={catalogue.crud.create}
	{toInput}
	{validate}
	blank={{ taxable: false }}
	bind:open={createOpen}
	onsaved={() => catalogue.query({ page: 1, limit: 50 })}
/>
