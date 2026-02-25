<script lang="ts">
	import Button from '$lib/components/shared/Button.svelte';
	import { getRateTiers } from '$lib/db/queries/rate-tiers.js';
	import { getColumnMappings, createColumnMapping, deleteColumnMapping } from '$lib/db/queries/column-mappings.js';
	import { autoDetectMapping, type ColumnMappingConfig, type TargetField } from '$lib/import/map-columns.js';
	import type { RateTier, ColumnMapping } from '$lib/types/index.js';

	let {
		headers,
		sampleRows,
		fileType,
		sheetName,
		headerRow,
		onmapped
	}: {
		headers: string[];
		sampleRows: Record<string, string>[];
		fileType: 'csv' | 'xlsx';
		sheetName: string;
		headerRow: number;
		onmapped: (config: ColumnMappingConfig) => void;
	} = $props();

	let tiers: RateTier[] = $state([]);
	let savedMappings: ColumnMapping[] = $state([]);
	let fieldMap: Record<string, TargetField> = $state({});
	let tierColumns: Record<string, number> = $state({});
	let newTierColumns: string[] = $state([]);
	let metadataColumns: string[] = $state([]);
	let saveName = $state('');
	let showSave = $state(false);

	// Initialize
	$effect(() => {
		tiers = getRateTiers();
		savedMappings = getColumnMappings('catalog');

		// Auto-detect mapping with smart data-driven heuristics
		const detected = autoDetectMapping(headers, sampleRows);
		const initial: Record<string, TargetField> = {};
		const initialNewTiers: string[] = [];
		const initialMetadata: string[] = [];

		for (const h of headers) {
			if (detected.fieldMap[h]) {
				initial[h] = detected.fieldMap[h]!;
			} else if (detected.suggestedNewTiers.includes(h)) {
				initialNewTiers.push(h);
			} else if (detected.suggestedMetadata.includes(h)) {
				initialMetadata.push(h);
			} else {
				// Default to metadata — user can explicitly skip
				initialMetadata.push(h);
			}
		}
		fieldMap = initial;
		tierColumns = {};
		newTierColumns = initialNewTiers;
		metadataColumns = initialMetadata;
	});

	function getMapping(header: string): string {
		if (tierColumns[header] !== undefined) return `tier:${tierColumns[header]}`;
		if (newTierColumns.includes(header)) return 'new_tier';
		if (metadataColumns.includes(header)) return 'metadata';
		return fieldMap[header] ?? 'skip';
	}

	function setMapping(header: string, value: string) {
		const newFieldMap = { ...fieldMap };
		const newTierCols = { ...tierColumns };
		let updatedNewTiers = newTierColumns.filter((c) => c !== header);
		let newMetadata = metadataColumns.filter((m) => m !== header);

		// Remove from all maps first
		delete newFieldMap[header];
		delete newTierCols[header];

		if (value === 'metadata') {
			newMetadata.push(header);
		} else if (value === 'new_tier') {
			updatedNewTiers.push(header);
		} else if (value.startsWith('tier:')) {
			newTierCols[header] = Number(value.split(':')[1]);
		} else {
			newFieldMap[header] = value as TargetField;
		}

		fieldMap = newFieldMap;
		tierColumns = newTierCols;
		newTierColumns = updatedNewTiers;
		metadataColumns = newMetadata;
	}

	function loadPreset(mapping: ColumnMapping) {
		try {
			const parsed = JSON.parse(mapping.mapping) as Record<string, string>;
			const parsedTiers = JSON.parse(mapping.tier_mapping || '{}') as Record<string, number>;
			const parsedMeta = JSON.parse(mapping.metadata_mapping || '[]') as string[];

			const newFieldMap: Record<string, TargetField> = {};
			for (const h of headers) {
				newFieldMap[h] = (parsed[h] as TargetField) ?? 'skip';
			}
			fieldMap = newFieldMap;

			const newTierCols: Record<string, number> = {};
			for (const [col, tierId] of Object.entries(parsedTiers)) {
				if (headers.includes(col)) {
					newTierCols[col] = tierId;
					delete fieldMap[col];
				}
			}
			tierColumns = newTierCols;

			metadataColumns = parsedMeta.filter((m) => headers.includes(m));
			for (const m of metadataColumns) {
				delete fieldMap[m];
			}

			newTierColumns = [];
		} catch {
			// Invalid mapping JSON
		}
	}

	async function savePreset() {
		if (!saveName.trim()) return;
		const allMappings: Record<string, string> = {};
		for (const h of headers) {
			allMappings[h] = fieldMap[h] ?? 'skip';
		}

		await createColumnMapping({
			name: saveName.trim(),
			entity_type: 'catalog',
			mapping: allMappings,
			tier_mapping: tierColumns,
			metadata_mapping: metadataColumns,
			file_type: fileType,
			sheet_name: sheetName,
			header_row: headerRow
		});

		savedMappings = getColumnMappings('catalog');
		saveName = '';
		showSave = false;
	}

	async function handleDeletePreset(id: number) {
		await deleteColumnMapping(id);
		savedMappings = getColumnMappings('catalog');
	}

	let hasNameMapping = $derived(
		Object.values(fieldMap).includes('name')
	);

	function handleNext() {
		onmapped({ fieldMap, tierColumns, newTierColumns, metadataColumns });
	}

	let preview = $derived(sampleRows.slice(0, 3));

	let newTierCount = $derived(newTierColumns.length);
</script>

<div class="space-y-4">
	<!-- Saved presets -->
	{#if savedMappings.length > 0}
		<div>
			<label class="block text-sm font-medium text-gray-700 mb-1">Saved mappings</label>
			<div class="flex flex-wrap gap-2">
				{#each savedMappings as mapping}
					<div class="inline-flex items-center gap-1 rounded-full bg-gray-100 px-3 py-1 text-sm">
						<button class="cursor-pointer text-primary-600 hover:text-primary-700" onclick={() => loadPreset(mapping)}>
							{mapping.name}
						</button>
						<button
							class="cursor-pointer ml-1 text-gray-400 hover:text-red-500"
							onclick={() => handleDeletePreset(mapping.id)}
							aria-label="Delete mapping"
						>
							&times;
						</button>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Column mapping table -->
	<div class="max-h-80 overflow-auto rounded-lg border border-gray-200">
		<table class="min-w-full divide-y divide-gray-200 text-sm">
			<thead class="sticky top-0 bg-gray-50">
				<tr>
					<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Source Column</th>
					<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Map To</th>
					<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500">Sample Values</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-200 bg-white">
				{#each headers as header}
					{@const mapping = getMapping(header)}
					<tr class={mapping === 'new_tier' ? 'bg-green-50' : ''}>
						<td class="whitespace-nowrap px-3 py-2 font-medium text-gray-900">{header}</td>
						<td class="px-3 py-2">
							<select
								value={mapping}
								onchange={(e) => setMapping(header, (e.target as HTMLSelectElement).value)}
								class="w-full rounded border border-gray-300 px-2 py-1 text-sm focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20"
							>
								<option value="skip">-- Skip --</option>
								<optgroup label="Standard Fields">
									<option value="name">Name</option>
									<option value="sku">SKU / Code</option>
									<option value="unit">Unit</option>
									<option value="category">Category</option>
									<option value="rate">Rate (Default)</option>
								</optgroup>
								<optgroup label="Rate Tiers">
									<option value="new_tier">+ Create as New Tier ("{header}")</option>
									{#each tiers as tier}
										<option value="tier:{tier.id}">{tier.name} Rate</option>
									{/each}
								</optgroup>
								<optgroup label="Other">
									<option value="metadata">Store as Metadata</option>
								</optgroup>
							</select>
						</td>
						<td class="px-3 py-2 text-gray-500">
							{preview.map((r) => r[header] || '').filter(Boolean).slice(0, 3).join(', ') || '-'}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	{#if !hasNameMapping}
		<div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700">
			A "Name" mapping is required. Please map at least one column to Name.
		</div>
	{/if}

	{#if newTierCount > 0}
		<div class="rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-700">
			<strong>{newTierCount} new rate tier{newTierCount === 1 ? '' : 's'}</strong> will be created on import:
			{newTierColumns.join(', ')}
		</div>
	{/if}

	<!-- Save preset -->
	<div class="flex items-center gap-2">
		{#if showSave}
			<input
				type="text"
				bind:value={saveName}
				placeholder="Mapping name..."
				class="rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			/>
			<Button variant="secondary" size="sm" onclick={savePreset} disabled={!saveName.trim()}>Save</Button>
			<Button variant="ghost" size="sm" onclick={() => (showSave = false)}>Cancel</Button>
		{:else}
			<Button variant="ghost" size="sm" onclick={() => (showSave = true)}>Save mapping as preset</Button>
		{/if}
	</div>

	<!-- Footer -->
	<div class="flex justify-end border-t border-gray-200 pt-4">
		<Button disabled={!hasNameMapping} onclick={handleNext}>
			Next
		</Button>
	</div>
</div>
