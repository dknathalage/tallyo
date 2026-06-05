<script lang="ts">
	import { onMount } from 'svelte';
	import { apiGet } from '$lib/api/client';
	import type { ColumnMapping, DiffResult, CommitResult } from '$lib/api/types';

	let mappings = $state<ColumnMapping[]>([]);
	let mappingsError = $state<string | null>(null);

	let mappingId = $state('');
	let files = $state<FileList | null>(null);

	let diff = $state<DiffResult | null>(null);
	let commitResult = $state<CommitResult | null>(null);
	let updateExisting = $state(false);

	let previewBusy = $state(false);
	let commitBusy = $state(false);
	let pageError = $state<string | null>(null);

	onMount(() => {
		void loadMappings();
	});

	async function loadMappings(): Promise<void> {
		mappingsError = null;
		try {
			const list = await apiGet<ColumnMapping[]>('/api/column-mappings?entityType=catalog');
			mappings = list ?? [];
		} catch (err) {
			mappingsError = err instanceof Error ? err.message : 'Failed to load mappings.';
			mappings = [];
		}
	}

	function selectedFile(): File | null {
		if (files === null || files.length === 0) return null;
		return files[0];
	}

	function buildFormData(file: File): FormData {
		const fd = new FormData();
		fd.append('file', file);
		fd.append('mappingId', String(mappingId));
		return fd;
	}

	async function preview(): Promise<void> {
		pageError = null;
		commitResult = null;
		const file = selectedFile();
		if (mappingId === '') {
			pageError = 'Please select a column mapping.';
			return;
		}
		if (file === null) {
			pageError = 'Please choose a file to import.';
			return;
		}
		previewBusy = true;
		try {
			const fd = buildFormData(file);
			const resp = await fetch('/api/import/catalog/preview', {
				method: 'POST',
				body: fd,
				credentials: 'include'
			});
			const text = await resp.text();
			const data: unknown = text.length > 0 ? JSON.parse(text) : null;
			if (!resp.ok) {
				const msg =
					(data as { error?: string } | null)?.error ?? `request failed (${resp.status})`;
				throw new Error(msg);
			}
			diff = data as DiffResult;
		} catch (err) {
			pageError = err instanceof Error ? err.message : 'Failed to preview import.';
			diff = null;
		} finally {
			previewBusy = false;
		}
	}

	async function commit(): Promise<void> {
		pageError = null;
		const file = selectedFile();
		if (mappingId === '') {
			pageError = 'Please select a column mapping.';
			return;
		}
		if (file === null) {
			pageError = 'Please choose a file to import.';
			return;
		}
		commitBusy = true;
		try {
			const fd = buildFormData(file);
			fd.append('updateExisting', String(updateExisting));
			const resp = await fetch('/api/import/catalog/commit', {
				method: 'POST',
				body: fd,
				credentials: 'include'
			});
			const text = await resp.text();
			const data: unknown = text.length > 0 ? JSON.parse(text) : null;
			if (!resp.ok) {
				const msg =
					(data as { error?: string } | null)?.error ?? `request failed (${resp.status})`;
				throw new Error(msg);
			}
			commitResult = data as CommitResult;
		} catch (err) {
			pageError = err instanceof Error ? err.message : 'Failed to commit import.';
		} finally {
			commitBusy = false;
		}
	}

	function reset(): void {
		diff = null;
		commitResult = null;
		updateExisting = false;
		pageError = null;
		files = null;
		mappingId = '';
	}

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}
</script>

<div class="space-y-8">
	<section>
		<h1 class="mb-1 text-xl font-semibold">Import Catalog</h1>
		<p class="mb-6 text-sm text-gray-500">
			Pick a column mapping and a CSV/Excel file, preview the diff, then commit.
		</p>

		{#if mappingsError}
			<p class="mb-3 text-sm text-red-600">{mappingsError}</p>
		{/if}
		{#if pageError}
			<p class="mb-3 text-sm text-red-600">{pageError}</p>
		{/if}

		<div class="space-y-4 rounded border border-gray-200 bg-white p-4">
			<div class="grid grid-cols-2 gap-3">
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Column mapping</span>
					<select
						bind:value={mappingId}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					>
						<option value="">— select —</option>
						{#each mappings as m (m.id)}
							<option value={String(m.id)}>{m.name} ({m.fileType})</option>
						{/each}
					</select>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">File</span>
					<input
						type="file"
						accept=".csv,.xlsx"
						bind:files
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
			</div>

			<button
				type="button"
				onclick={preview}
				disabled={previewBusy}
				class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{previewBusy ? 'Previewing…' : 'Preview'}
			</button>
		</div>
	</section>

	{#if diff}
		<section>
			<h2 class="mb-3 text-base font-semibold">Preview</h2>

			<dl class="mb-4 grid max-w-xl grid-cols-5 gap-2 text-center text-sm">
				<div class="rounded border border-gray-200 bg-white p-2">
					<dt class="text-gray-500">Total</dt>
					<dd class="font-semibold">{diff.summary.total}</dd>
				</div>
				<div class="rounded border border-gray-200 bg-white p-2">
					<dt class="text-gray-500">New</dt>
					<dd class="font-semibold">{diff.summary.new}</dd>
				</div>
				<div class="rounded border border-gray-200 bg-white p-2">
					<dt class="text-gray-500">Updated</dt>
					<dd class="font-semibold">{diff.summary.updated}</dd>
				</div>
				<div class="rounded border border-gray-200 bg-white p-2">
					<dt class="text-gray-500">Unchanged</dt>
					<dd class="font-semibold">{diff.summary.unchanged}</dd>
				</div>
				<div class="rounded border border-gray-200 bg-white p-2">
					<dt class="text-gray-500">Errors</dt>
					<dd class="font-semibold">{diff.summary.errors}</dd>
				</div>
			</dl>

			<p class="mb-2 text-sm text-gray-500">
				New items (showing first {Math.min(diff.new.length, 10)} of {diff.new.length}); updated
				items: {diff.updated.length}.
			</p>

			<div class="mb-6 overflow-hidden rounded border border-gray-200 bg-white">
				<table class="w-full text-sm">
					<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
						<tr>
							<th class="px-3 py-2 font-medium">Name</th>
							<th class="px-3 py-2 font-medium">SKU</th>
							<th class="px-3 py-2 font-medium text-right">Rate</th>
						</tr>
					</thead>
					<tbody>
						{#each diff.new.slice(0, 10) as row, i (i)}
							<tr class="border-b border-gray-100 last:border-0">
								<td class="px-3 py-2 font-medium">{row.name}</td>
								<td class="px-3 py-2 text-gray-600">{row.sku || '—'}</td>
								<td class="px-3 py-2 text-right">{money(row.rate)}</td>
							</tr>
						{:else}
							<tr>
								<td colspan="3" class="px-3 py-4 text-center text-gray-500">
									No new items.
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			<div class="space-y-3 rounded border border-gray-200 bg-white p-4">
				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={updateExisting} />
					<span>Update existing items</span>
				</label>

				<div class="flex gap-2">
					<button
						type="button"
						onclick={commit}
						disabled={commitBusy}
						class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
					>
						{commitBusy ? 'Importing…' : 'Commit import'}
					</button>
					<button
						type="button"
						onclick={reset}
						class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
					>
						Reset
					</button>
				</div>
			</div>
		</section>
	{/if}

	{#if commitResult}
		<section>
			<div class="rounded border border-green-200 bg-green-50 p-4 text-sm">
				<p class="mb-2 font-semibold text-green-800">Import complete.</p>
				<p class="text-green-700">
					Inserted {commitResult.inserted}, updated {commitResult.updated}.
				</p>
				<button
					type="button"
					onclick={reset}
					class="mt-3 rounded border border-gray-300 bg-white px-4 py-2 text-sm hover:bg-gray-50"
				>
					Start another import
				</button>
			</div>
		</section>
	{/if}
</div>
