<script lang="ts">
	import { onMount } from 'svelte';
	import { columnMappings } from '$lib/stores/columnMappings.svelte';
	import type { ColumnMapping } from '$lib/api/types';

	const FILE_TYPES = ['csv', 'xlsx'];

	// Form state (shared by create + edit).
	let editId = $state<number | null>(null);
	let formName = $state('');
	let formEntityType = $state('catalog');
	let formFileType = $state('csv');
	let formSheetName = $state('');
	let formHeaderRow = $state(1);
	let formMapping = $state('{}');
	let formTierMapping = $state('{}');
	let formMetadataMapping = $state('[]');
	let saving = $state(false);
	let formError = $state<string | null>(null);
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	onMount(() => {
		columnMappings.ensureSubscribed();
		void columnMappings.load();
	});

	function resetForm(): void {
		editId = null;
		formName = '';
		formEntityType = 'catalog';
		formFileType = 'csv';
		formSheetName = '';
		formHeaderRow = 1;
		formMapping = '{}';
		formTierMapping = '{}';
		formMetadataMapping = '[]';
		formError = null;
	}

	// Validate that the given text parses as JSON of the expected kind.
	function validateJson(text: string, kind: 'object' | 'array'): string | null {
		if (typeof text !== 'string') {
			return 'value must be a string';
		}
		let parsed: unknown;
		try {
			parsed = JSON.parse(text);
		} catch {
			return 'invalid JSON';
		}
		if (kind === 'array' && !Array.isArray(parsed)) {
			return 'expected a JSON array';
		}
		if (kind === 'object' && (Array.isArray(parsed) || typeof parsed !== 'object' || parsed === null)) {
			return 'expected a JSON object';
		}
		return null;
	}

	async function submitForm(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		if (formName.trim() === '') {
			formError = 'Please enter a name.';
			return;
		}
		const mErr = validateJson(formMapping, 'object');
		if (mErr !== null) {
			formError = `Mapping: ${mErr}`;
			return;
		}
		const tErr = validateJson(formTierMapping, 'object');
		if (tErr !== null) {
			formError = `Tier mapping: ${tErr}`;
			return;
		}
		const dErr = validateJson(formMetadataMapping, 'array');
		if (dErr !== null) {
			formError = `Metadata mapping: ${dErr}`;
			return;
		}
		saving = true;
		try {
			const payload = {
				name: formName,
				entityType: formEntityType,
				mapping: formMapping,
				tierMapping: formTierMapping,
				metadataMapping: formMetadataMapping,
				fileType: formFileType,
				sheetName: formSheetName,
				headerRow: Number(formHeaderRow)
			};
			if (editId === null) {
				await columnMappings.crud.create(payload);
			} else {
				await columnMappings.crud.update(editId, payload);
			}
			resetForm();
			await columnMappings.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to save mapping.';
		} finally {
			saving = false;
		}
	}

	function startEdit(m: ColumnMapping): void {
		formError = null;
		editId = m.id;
		formName = m.name;
		formEntityType = m.entityType;
		formFileType = m.fileType;
		formSheetName = m.sheetName;
		formHeaderRow = m.headerRow;
		formMapping = m.mapping;
		formTierMapping = m.tierMapping;
		formMetadataMapping = m.metadataMapping;
	}

	async function remove(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await columnMappings.crud.remove(id);
			if (editId === id) {
				resetForm();
			}
			await columnMappings.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete mapping.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<h1 class="mb-1 text-xl font-semibold">Column Mappings</h1>
		<p class="mb-6 text-sm text-gray-500">
			Define how imported CSV/Excel columns map onto fields. Mapping and tier mapping are JSON
			objects; metadata mapping is a JSON array.
		</p>

		<form class="space-y-4 rounded border border-gray-200 bg-white p-4" onsubmit={submitForm}>
			<h2 class="text-base font-semibold">
				{editId === null ? 'New mapping' : 'Edit mapping'}
			</h2>

			<div class="grid grid-cols-2 gap-3">
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Name</span>
					<input
						type="text"
						bind:value={formName}
						required
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Entity type</span>
					<input
						type="text"
						bind:value={formEntityType}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">File type</span>
					<select
						bind:value={formFileType}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					>
						{#each FILE_TYPES as ft (ft)}
							<option value={ft}>{ft}</option>
						{/each}
					</select>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Sheet name</span>
					<input
						type="text"
						bind:value={formSheetName}
						placeholder="(xlsx only)"
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Header row</span>
					<input
						type="number"
						min="1"
						bind:value={formHeaderRow}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
			</div>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Mapping (JSON object)</span>
				<textarea
					bind:value={formMapping}
					rows="4"
					class="w-full rounded border border-gray-300 px-3 py-2 font-mono text-xs"
				></textarea>
			</label>
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Tier mapping (JSON object)</span>
				<textarea
					bind:value={formTierMapping}
					rows="3"
					class="w-full rounded border border-gray-300 px-3 py-2 font-mono text-xs"
				></textarea>
			</label>
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Metadata mapping (JSON array)</span>
				<textarea
					bind:value={formMetadataMapping}
					rows="3"
					class="w-full rounded border border-gray-300 px-3 py-2 font-mono text-xs"
				></textarea>
			</label>

			{#if formError}
				<p class="text-sm text-red-600">{formError}</p>
			{/if}

			<div class="flex gap-2">
				<button
					type="submit"
					disabled={saving}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{saving ? 'Saving…' : editId === null ? 'Create mapping' : 'Save changes'}
				</button>
				{#if editId !== null}
					<button
						type="button"
						onclick={resetForm}
						class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
					>
						Cancel
					</button>
				{/if}
			</div>
		</form>
	</section>

	<section>
		{#if columnMappings.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if columnMappings.error}
			<p class="text-sm text-red-600">{columnMappings.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}

		<div class="overflow-hidden rounded border border-gray-200 bg-white">
			<table class="w-full text-sm">
				<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
					<tr>
						<th class="px-3 py-2 font-medium">Name</th>
						<th class="px-3 py-2 font-medium">Entity</th>
						<th class="px-3 py-2 font-medium">File type</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each columnMappings.items as m (m.id)}
						<tr class="border-b border-gray-100 last:border-0">
							<td class="px-3 py-2 font-medium">{m.name}</td>
							<td class="px-3 py-2 text-gray-600">{m.entityType}</td>
							<td class="px-3 py-2 text-gray-600">{m.fileType}</td>
							<td class="px-3 py-2 text-right whitespace-nowrap">
								<button
									type="button"
									onclick={() => startEdit(m)}
									class="mr-2 text-gray-900 hover:underline"
								>
									Edit
								</button>
								<button
									type="button"
									disabled={busy}
									onclick={() => remove(m.id)}
									class="text-red-600 hover:underline disabled:opacity-50"
								>
									Delete
								</button>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="4" class="px-3 py-6 text-center text-gray-500">
								No column mappings found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
