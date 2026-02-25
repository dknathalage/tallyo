<script lang="ts">
	import Modal from '$lib/components/shared/Modal.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import StepFileSelect from './StepFileSelect.svelte';
	import StepColumnMapping from './StepColumnMapping.svelte';
	import StepImportMode from './StepImportMode.svelte';
	import StepPreviewDiff from './StepPreviewDiff.svelte';
	import { applyMapping, type ColumnMappingConfig } from '$lib/import/map-columns.js';
	import { diffCatalog, type DiffResult } from '$lib/import/diff-catalog.js';
	import { commitCatalogImport } from '$lib/import/commit-catalog.js';
	import { getCatalogItems } from '$lib/db/queries/catalog.js';
	import { createRateTier } from '$lib/db/queries/rate-tiers.js';
	import type { ParsedFile, ParsedSheet } from '$lib/import/parse-file.js';

	let {
		open,
		onclose,
		oncomplete
	}: {
		open: boolean;
		onclose: () => void;
		oncomplete: () => void;
	} = $props();

	let currentStep = $state(1);
	let parsedFile: ParsedFile | null = $state(null);
	let activeSheet: ParsedSheet | null = $state(null);
	let mappingConfig: ColumnMappingConfig | null = $state(null);
	let importMode: 'insert_only' | 'upsert' = $state('upsert');
	let diffResult: DiffResult | null = $state(null);
	let committing = $state(false);
	let error = $state('');

	const STEPS = ['File', 'Mapping', 'Mode', 'Preview'] as const;

	function stepTitle(): string {
		switch (currentStep) {
			case 1: return 'Import Catalog - Select File';
			case 2: return 'Import Catalog - Map Columns';
			case 3: return 'Import Catalog - Import Mode';
			case 4: return 'Import Catalog - Preview';
			default: return 'Import Catalog';
		}
	}

	function reset() {
		currentStep = 1;
		parsedFile = null;
		activeSheet = null;
		mappingConfig = null;
		importMode = 'upsert';
		diffResult = null;
		committing = false;
		error = '';
	}

	function handleClose() {
		reset();
		onclose();
	}

	function handleFileParsed(file: ParsedFile, sheet: ParsedSheet) {
		parsedFile = file;
		activeSheet = sheet;
		currentStep = 2;
	}

	function handleMapped(config: ColumnMappingConfig) {
		mappingConfig = config;
		currentStep = 3;
	}

	async function handleModeSelected(mode: 'insert_only' | 'upsert') {
		importMode = mode;

		if (!activeSheet || !mappingConfig) return;

		// Create any new tiers first, then replace newTierColumns with real tier IDs
		if (mappingConfig.newTierColumns.length > 0) {
			const resolvedTierColumns = { ...mappingConfig.tierColumns };
			for (const colName of mappingConfig.newTierColumns) {
				const tierId = await createRateTier({
					name: colName,
					description: `Auto-created from import column "${colName}"`
				});
				resolvedTierColumns[colName] = tierId;
			}
			mappingConfig = {
				...mappingConfig,
				tierColumns: resolvedTierColumns,
				newTierColumns: []
			};
		}

		// Run diff
		const mapped = applyMapping(activeSheet.rows, mappingConfig);
		const existing = getCatalogItems().map((item) => ({
			id: item.id,
			name: item.name,
			sku: item.sku,
			rate: item.rate,
			unit: item.unit,
			category: item.category
		}));
		diffResult = diffCatalog(mapped, existing);
		currentStep = 4;
	}

	async function handleCommit() {
		if (!diffResult) return;
		committing = true;
		error = '';
		try {
			await commitCatalogImport(diffResult, { updateExisting: importMode === 'upsert' });
			reset();
			oncomplete();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Import failed';
		} finally {
			committing = false;
		}
	}

	function goBack() {
		if (currentStep > 1) {
			currentStep--;
		}
	}
</script>

<Modal {open} onclose={handleClose} title={stepTitle()} maxWidth="max-w-4xl">
	<div class="space-y-4">
		<!-- Step indicator -->
		<div class="flex items-center justify-center gap-1">
			{#each STEPS as step, i}
				{@const stepNum = i + 1}
				<div class="flex items-center gap-1">
					<div
						class="flex h-7 w-7 items-center justify-center rounded-full text-xs font-medium {
							stepNum < currentStep
								? 'bg-primary-600 text-white'
								: stepNum === currentStep
									? 'bg-primary-100 text-primary-700 ring-2 ring-primary-500'
									: 'bg-gray-100 text-gray-400'
						}"
					>
						{#if stepNum < currentStep}
							<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor">
								<path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
							</svg>
						{:else}
							{stepNum}
						{/if}
					</div>
					<span class="hidden text-xs sm:inline {stepNum === currentStep ? 'font-medium text-gray-900' : 'text-gray-400'}">
						{step}
					</span>
					{#if i < STEPS.length - 1}
						<div class="mx-2 h-px w-8 {stepNum < currentStep ? 'bg-primary-400' : 'bg-gray-200'}"></div>
					{/if}
				</div>
			{/each}
		</div>

		{#if error}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div>
		{/if}

		<!-- Step content -->
		{#if currentStep === 1}
			<StepFileSelect onparsed={handleFileParsed} />
		{:else if currentStep === 2 && activeSheet && parsedFile}
			<div>
				<div class="mb-4">
					<Button variant="ghost" size="sm" onclick={goBack}>
						&larr; Back
					</Button>
				</div>
				<StepColumnMapping
					headers={activeSheet.headers}
					sampleRows={activeSheet.rows.slice(0, 100)}
					fileType={parsedFile.fileType}
					sheetName={activeSheet.sheetName}
					headerRow={1}
					onmapped={handleMapped}
				/>
			</div>
		{:else if currentStep === 3}
			<div>
				<div class="mb-4">
					<Button variant="ghost" size="sm" onclick={goBack}>
						&larr; Back
					</Button>
				</div>
				<StepImportMode onselect={handleModeSelected} />
			</div>
		{:else if currentStep === 4 && diffResult}
			<div>
				<div class="mb-4">
					<Button variant="ghost" size="sm" onclick={goBack}>
						&larr; Back
					</Button>
				</div>
				<StepPreviewDiff
					diff={diffResult}
					{importMode}
					oncommit={handleCommit}
					loading={committing}
				/>
			</div>
		{/if}
	</div>
</Modal>
