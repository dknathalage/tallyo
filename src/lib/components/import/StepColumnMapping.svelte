<script lang="ts">
	import { onMount } from 'svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import { autoDetectMapping, type ColumnMappingConfig, type TargetField } from '$lib/import/map-columns.js';
	import type { RateTier, ColumnMapping } from '$lib/types/index.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	const {
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
	onMount(async () => {
		const [tiersRes, mappingsRes] = await Promise.all([
			fetch('/api/rate-tiers'),
			fetch('/api/column-mappings?entity=catalog')
		]);
		tiers = await tiersRes.json();
		savedMappings = await mappingsRes.json();
	});

	$effect(() => {

		// Auto-detect mapping with smart data-driven heuristics
		const detected = autoDetectMapping(headers, sampleRows);
		const initial: Record<string, TargetField> = {};
		const initialNewTiers: string[] = [];
		const initialMetadata: string[] = [];

		for (const h of headers) {
			const mapped = detected.fieldMap[h];
			if (mapped) {
				initial[h] = mapped;
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
		const updatedNewTiers = newTierColumns.filter((c) => c !== header);
		const newMetadata = metadataColumns.filter((m) => m !== header);

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
			newFieldMap[header] = value;
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
				newFieldMap[h] = parsed[h] ?? 'skip';
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
		const name = saveName.trim();
		if (!name) return;
		saveName = '';
		showSave = false;
		const allMappings: Record<string, string> = {};
		for (const h of headers) {
			allMappings[h] = fieldMap[h] ?? 'skip';
		}

		await fetch('/api/column-mappings', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				name,
				entity_type: 'catalog',
				mapping: allMappings,
				tier_mapping: tierColumns,
				metadata_mapping: metadataColumns,
				file_type: fileType,
				sheet_name: sheetName,
				header_row: headerRow
			})
		});

		const res = await fetch('/api/column-mappings?entity=catalog');
		savedMappings = await res.json();
	}

	async function handleDeletePreset(id: number) {
		await fetch(`/api/column-mappings?id=${id}`, { method: 'DELETE' });
		const res = await fetch('/api/column-mappings?entity=catalog');
		savedMappings = await res.json();
	}

	const hasNameMapping = $derived(
		Object.values(fieldMap).includes('name')
	);

	function handleNext() {
		onmapped({ fieldMap, tierColumns, newTierColumns, metadataColumns });
	}

	const preview = $derived(sampleRows.slice(0, 3));

	const newTierCount = $derived(newTierColumns.length);
</script>

<div class="space-y-4">
	<!-- Saved presets -->
	{#if savedMappings.length > 0}
		<div>
			<div class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">{i18n.t('importWizard.savedMappings')}</div>
			<div class="flex flex-wrap gap-2">
				{#each savedMappings as mapping}
					<div class="inline-flex items-center gap-1 rounded-full bg-gray-100 dark:bg-gray-700 px-3 py-1 text-sm">
						<button class="cursor-pointer text-primary-600 hover:text-primary-700" onclick={() => loadPreset(mapping)}>
							{mapping.name}
						</button>
						<button
							class="cursor-pointer ml-1 text-gray-400 dark:text-gray-500 hover:text-red-500"
							onclick={() => handleDeletePreset(mapping.id)}
							aria-label={i18n.t('importWizard.deleteMapping')}
						>
							&times;
						</button>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Column mapping table -->
	<div class="max-h-80 overflow-auto rounded-lg border border-gray-200 dark:border-gray-700">
		<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
			<thead class="sticky top-0 bg-gray-50 dark:bg-gray-900">
				<tr>
					<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('importWizard.sourceColumn')}</th>
					<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('importWizard.mapTo')}</th>
					<th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{i18n.t('importWizard.sampleValues')}</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-800">
				{#each headers as header}
					{@const mapping = getMapping(header)}
					<tr class={mapping === 'new_tier' ? 'bg-green-50 dark:bg-green-900/20' : ''}>
						<td class="whitespace-nowrap px-3 py-2 font-medium text-gray-900 dark:text-white">{header}</td>
						<td class="px-3 py-2">
							<select
								value={mapping}
								onchange={(e) => setMapping(header, (e.target as HTMLSelectElement).value)}
								class="w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-2 py-1 text-sm dark:text-white focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20"
							>
								<option value="skip">{i18n.t('importWizard.skip')}</option>
								<optgroup label={i18n.t('importWizard.standardFields')}>
									<option value="name">{i18n.t('catalog.name')}</option>
									<option value="sku">{i18n.t('catalog.sku')} / Code</option>
									<option value="unit">{i18n.t('catalog.unit')}</option>
									<option value="category">{i18n.t('catalog.category')}</option>
									<option value="rate">{i18n.t('catalog.rate')} ({i18n.t('common.default')})</option>
								</optgroup>
								<optgroup label={i18n.t('importWizard.rateTiersGroup')}>
									<option value="new_tier">{i18n.t('importWizard.createAsNewTier', { name: header })}</option>
									{#each tiers as tier}
										<option value="tier:{tier.id}">{tier.name} {i18n.t('catalog.rate')}</option>
									{/each}
								</optgroup>
								<optgroup label={i18n.t('importWizard.otherGroup')}>
									<option value="metadata">{i18n.t('importWizard.storeAsMetadata')}</option>
								</optgroup>
							</select>
						</td>
						<td class="px-3 py-2 text-gray-500 dark:text-gray-400">
							{preview.map((r) => r[header] ?? '').filter(Boolean).slice(0, 3).join(', ') || '-'}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	{#if !hasNameMapping}
		<div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700">
			{i18n.t('importWizard.nameRequired')}
		</div>
	{/if}

	{#if newTierCount > 0}
		<div class="rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-700">
			{i18n.t('importWizard.newTiersCreated', { count: String(newTierCount), plural: newTierCount === 1 ? '' : 's', names: newTierColumns.join(', ') })}
		</div>
	{/if}

	<!-- Save preset -->
	<div class="flex items-center gap-2">
		{#if showSave}
			<input
				type="text"
				bind:value={saveName}
				placeholder={i18n.t('importWizard.mappingNamePlaceholder')}
				class="rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-1.5 text-sm dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			/>
			<Button variant="secondary" size="sm" onclick={savePreset} disabled={!saveName.trim()}>{i18n.t('common.save')}</Button>
			<Button variant="ghost" size="sm" onclick={() => (showSave = false)}>{i18n.t('common.cancel')}</Button>
		{:else}
			<Button variant="ghost" size="sm" onclick={() => (showSave = true)}>{i18n.t('importWizard.saveMappingPreset')}</Button>
		{/if}
	</div>

	<!-- Footer -->
	<div class="flex justify-end border-t border-gray-200 dark:border-gray-700 pt-4">
		<Button disabled={!hasNameMapping} onclick={handleNext}>
			{i18n.t('common.next')}
		</Button>
	</div>
</div>
