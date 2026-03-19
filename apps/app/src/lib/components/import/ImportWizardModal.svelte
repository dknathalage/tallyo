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
	import type { ParsedFile, ParsedSheet } from '$lib/import/parse-file.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';

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

	let STEPS = $derived([i18n.t('importWizard.file'), i18n.t('importWizard.mapping'), i18n.t('importWizard.mode'), i18n.t('importWizard.preview')] as const);

	function stepTitle(): string {
		switch (currentStep) {
			case 1: return i18n.t('importWizard.selectFile');
			case 2: return i18n.t('importWizard.mapColumns');
			case 3: return i18n.t('importWizard.importMode');
			case 4: return i18n.t('importWizard.preview');
			default: return i18n.t('importWizard.title');
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
				const res = await fetch('/api/rate-tiers', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ name: colName, description: `Auto-created from import column "${colName}"` })
				});
				const { id: tierId } = await res.json();
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
		const existingRes = await fetch('/api/catalog');
		const existingItems = await existingRes.json();
		const existing = existingItems.map((item: { id: number; name: string; sku: string; rate: number; unit: string; category: string }) => ({
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
									? 'bg-primary-100 dark:bg-primary-900/50 text-primary-700 dark:text-primary-300 ring-2 ring-primary-500'
									: 'bg-gray-100 dark:bg-gray-700 text-gray-400 dark:text-gray-500'
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
					<span class="hidden text-xs sm:inline {stepNum === currentStep ? 'font-medium text-gray-900 dark:text-white' : 'text-gray-400 dark:text-gray-500'}">
						{step}
					</span>
					{#if i < STEPS.length - 1}
						<div class="mx-2 h-px w-8 {stepNum < currentStep ? 'bg-primary-400' : 'bg-gray-200 dark:bg-gray-700'}"></div>
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
						&larr; {i18n.t('common.back')}
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
						&larr; {i18n.t('common.back')}
					</Button>
				</div>
				<StepImportMode onselect={handleModeSelected} />
			</div>
		{:else if currentStep === 4 && diffResult}
			<div>
				<div class="mb-4">
					<Button variant="ghost" size="sm" onclick={goBack}>
						&larr; {i18n.t('common.back')}
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
