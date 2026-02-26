<script lang="ts">
	import Button from '$lib/components/shared/Button.svelte';
	import { parseFile, getSheetWithHeaderRow, type ParsedFile, type ParsedSheet } from '$lib/import/parse-file.js';

	let {
		onparsed
	}: {
		onparsed: (file: ParsedFile, selectedSheet: ParsedSheet) => void;
	} = $props();

	let fileInput: HTMLInputElement;
	let parsedFile: ParsedFile | null = $state(null);
	let selectedSheetIndex = $state(0);
	let headerRow = $state(1);
	let loading = $state(false);
	let error = $state('');

	let activeSheet: ParsedSheet | null = $derived.by(() => {
		if (!parsedFile || parsedFile.sheets.length === 0) return null;
		const raw = parsedFile.sheets[selectedSheetIndex];
		if (!raw) return null;
		if (parsedFile.fileType === 'xlsx' && headerRow > 1) {
			return getSheetWithHeaderRow(raw, headerRow);
		}
		return raw;
	});

	let previewRows = $derived(activeSheet?.rows.slice(0, 5) ?? []);

	async function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files?.[0]) return;

		loading = true;
		error = '';
		try {
			parsedFile = await parseFile(input.files[0]);
			selectedSheetIndex = 0;
			headerRow = 1;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to parse file';
			parsedFile = null;
		} finally {
			loading = false;
		}
	}

	function handleNext() {
		if (parsedFile && activeSheet) {
			onparsed(parsedFile, activeSheet);
		}
	}
</script>

<div class="space-y-4">
	<!-- File input -->
	<div>
		<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Select file to import</label>
		<div class="flex items-center gap-3">
			<Button variant="secondary" size="sm" onclick={() => fileInput.click()}>
				Choose File
			</Button>
			{#if parsedFile}
				<span class="text-sm text-gray-600 dark:text-gray-300">{parsedFile.fileName}</span>
			{:else}
				<span class="text-sm text-gray-400 dark:text-gray-500">No file selected</span>
			{/if}
		</div>
		<input
			bind:this={fileInput}
			type="file"
			accept=".csv,.xlsx,.xls"
			class="hidden"
			onchange={handleFileSelect}
		/>
	</div>

	{#if loading}
		<div class="text-sm text-gray-500 dark:text-gray-400">Parsing file...</div>
	{/if}

	{#if error}
		<div class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div>
	{/if}

	{#if parsedFile}
		<!-- Sheet selector (Excel only) -->
		{#if parsedFile.fileType === 'xlsx' && parsedFile.sheets.length > 1}
			<div>
				<label for="sheet-select" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Sheet</label>
				<select
					id="sheet-select"
					bind:value={selectedSheetIndex}
					class="rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				>
					{#each parsedFile.sheets as sheet, i}
						<option value={i}>{sheet.sheetName} ({sheet.rows.length} rows)</option>
					{/each}
				</select>
			</div>
		{/if}

		<!-- Header row (Excel only) -->
		{#if parsedFile.fileType === 'xlsx'}
			<div>
				<label for="header-row" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Header row</label>
				<input
					id="header-row"
					type="number"
					min="1"
					bind:value={headerRow}
					class="w-24 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				/>
				<span class="ml-2 text-xs text-gray-500 dark:text-gray-400">Row number containing column headers</span>
			</div>
		{/if}

		<!-- Preview -->
		{#if activeSheet}
			<div>
				<p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
					Preview: {activeSheet.headers.length} columns, {activeSheet.rows.length} rows
					{#if activeSheet.rows.length > 5}
						(showing first 5)
					{/if}
				</p>
				<div class="max-h-64 overflow-auto rounded-lg border border-gray-200 dark:border-gray-700">
					<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
						<thead class="sticky top-0 bg-gray-50 dark:bg-gray-900">
							<tr>
								{#each activeSheet.headers as header}
									<th class="whitespace-nowrap px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
										{header}
									</th>
								{/each}
							</tr>
						</thead>
						<tbody class="divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-800">
							{#each previewRows as row}
								<tr>
									{#each activeSheet.headers as header}
										<td class="whitespace-nowrap px-3 py-1.5 text-gray-700 dark:text-gray-300">
											{row[header] || ''}
										</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}
	{/if}

	<!-- Footer -->
	<div class="flex justify-end border-t border-gray-200 dark:border-gray-700 pt-4">
		<Button disabled={!activeSheet || activeSheet.rows.length === 0} onclick={handleNext}>
			Next
		</Button>
	</div>
</div>
