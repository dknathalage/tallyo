<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { rateTiers } from '$lib/stores/rateTiers.svelte';
	import type { ImportParseResult, DiffResult, CommitResult } from '$lib/api/types';

	// Sentinel select values for the tier dropdowns.
	const NEW_TIER = '__new__';
	const IGNORE_TIER = '__ignore__';

	// Field options for header → catalog field mapping.
	const FIELD_OPTIONS = ['ignore', 'name', 'sku', 'unit', 'category', 'rate'] as const;

	type Step = 'upload' | 'review' | 'diff';
	let step = $state<Step>('upload');

	let files = $state<FileList | null>(null);

	let parsed = $state<ImportParseResult | null>(null);
	let fieldByHeader = $state<Record<string, string>>({});
	// Per price-column header: chosen tier value (existing tier id as string,
	// NEW_TIER, or IGNORE_TIER) and the editable new-tier name.
	let tierChoiceByHeader = $state<Record<string, string>>({});
	let newTierNameByHeader = $state<Record<string, string>>({});

	let diff = $state<DiffResult | null>(null);
	let commitResult = $state<CommitResult | null>(null);
	let updateExisting = $state(false);

	let parseBusy = $state(false);
	let previewBusy = $state(false);
	let commitBusy = $state(false);
	let pageError = $state<string | null>(null);

	onMount(() => {
		rateTiers.ensureSubscribed();
		void rateTiers.load();
	});

	function selectedFile(): File | null {
		if (files === null || files.length === 0) return null;
		return files[0];
	}

	/** POST a multipart form; raw fetch since the JSON client can't do FormData. */
	async function postForm<T>(path: string, fd: FormData): Promise<T> {
		const resp = await fetch(path, { method: 'POST', body: fd, credentials: 'include' });
		const text = await resp.text();
		const data: unknown = text.length > 0 ? JSON.parse(text) : null;
		if (!resp.ok) {
			const msg = (data as { error?: string } | null)?.error ?? `request failed (${resp.status})`;
			throw new Error(msg);
		}
		return data as T;
	}

	async function parse(): Promise<void> {
		pageError = null;
		const file = selectedFile();
		if (file === null) {
			pageError = 'Please choose a file to import.';
			return;
		}
		parseBusy = true;
		try {
			const fd = new FormData();
			fd.append('file', file);
			const res = await postForm<ImportParseResult>('/api/catalog/import/parse', fd);
			parsed = res;
			seedReview(res);
			step = 'review';
		} catch (err) {
			pageError = err instanceof Error ? err.message : 'Failed to parse file.';
		} finally {
			parseBusy = false;
		}
	}

	/** Pre-fill review state from the server's suggestion. */
	function seedReview(res: ImportParseResult): void {
		const fields: Record<string, string> = {};
		for (const h of res.headers) {
			fields[h] = res.suggestion.fields[h] ?? 'ignore';
		}
		fieldByHeader = fields;

		const choices: Record<string, string> = {};
		const names: Record<string, string> = {};
		for (const pc of res.suggestion.priceCols) {
			const match = rateTiers.items.find(
				(t) => t.name.toLowerCase() === pc.suggestName.toLowerCase()
			);
			if (match !== undefined) {
				choices[pc.header] = String(match.id);
			} else {
				choices[pc.header] = NEW_TIER;
			}
			names[pc.header] = pc.suggestName;
		}
		tierChoiceByHeader = choices;
		newTierNameByHeader = names;
	}

	const hasName = $derived.by<boolean>(() =>
		Object.values(fieldByHeader).some((v) => v === 'name')
	);

	/** Build the transient mapping JSON for preview/commit. */
	function buildMapping(): string {
		const fields: Record<string, string> = {};
		for (const [header, field] of Object.entries(fieldByHeader)) {
			if (field !== 'ignore') fields[header] = field;
		}
		const tierCols: Record<string, string> = {};
		for (const [header, choice] of Object.entries(tierChoiceByHeader)) {
			if (choice === IGNORE_TIER) continue;
			if (choice === NEW_TIER) {
				const name = (newTierNameByHeader[header] ?? '').trim();
				if (name !== '') tierCols[header] = name;
				continue;
			}
			const tier = rateTiers.items.find((t) => String(t.id) === choice);
			if (tier !== undefined) tierCols[header] = tier.name;
		}
		return JSON.stringify({ fields, tierCols, fileType: '', sheetName: '', headerRow: 1 });
	}

	async function preview(): Promise<void> {
		pageError = null;
		const file = selectedFile();
		if (file === null) {
			pageError = 'Please choose a file to import.';
			return;
		}
		if (!hasName) {
			pageError = 'Map at least one column to "name" before previewing.';
			return;
		}
		previewBusy = true;
		try {
			const fd = new FormData();
			fd.append('file', file);
			fd.append('mapping', buildMapping());
			diff = await postForm<DiffResult>('/api/catalog/import/preview', fd);
			step = 'diff';
		} catch (err) {
			pageError = err instanceof Error ? err.message : 'Failed to preview import.';
		} finally {
			previewBusy = false;
		}
	}

	async function commit(): Promise<void> {
		pageError = null;
		const file = selectedFile();
		if (file === null) {
			pageError = 'Please choose a file to import.';
			return;
		}
		commitBusy = true;
		try {
			const fd = new FormData();
			fd.append('file', file);
			fd.append('mapping', buildMapping());
			fd.append('updateExisting', String(updateExisting));
			commitResult = await postForm<CommitResult>('/api/catalog/import/commit', fd);
			await goto('/catalog');
		} catch (err) {
			pageError = err instanceof Error ? err.message : 'Failed to commit import.';
		} finally {
			commitBusy = false;
		}
	}

	function backToUpload(): void {
		pageError = null;
		parsed = null;
		diff = null;
		step = 'upload';
	}

	function backToReview(): void {
		pageError = null;
		diff = null;
		step = 'review';
	}

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}
</script>

<div class="space-y-8">
	<section>
		<div class="mb-6 flex items-start justify-between">
			<div>
				<h1 class="mb-1 text-xl font-semibold">Import Catalog</h1>
				<p class="text-sm text-gray-500">
					Upload a CSV/Excel file, map its columns, preview the diff, then commit.
				</p>
			</div>
			<a
				href="/catalog"
				class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
			>
				Back to catalog
			</a>
		</div>

		{#if pageError}
			<p class="mb-3 text-sm text-red-600">{pageError}</p>
		{/if}
	</section>

	{#if step === 'upload'}
		<section class="space-y-4 rounded border border-gray-200 bg-white p-4">
			<label class="block max-w-md">
				<span class="mb-1 block text-sm font-medium">File</span>
				<input
					type="file"
					accept=".csv,.xlsx"
					bind:files
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<button
				type="button"
				onclick={parse}
				disabled={parseBusy}
				class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{parseBusy ? 'Parsing…' : 'Continue'}
			</button>
		</section>
	{/if}

	{#if step === 'review' && parsed}
		<section class="space-y-6">
			<div class="overflow-hidden rounded border border-gray-200 bg-white">
				<table class="w-full text-sm">
					<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
						<tr>
							<th class="px-3 py-2 font-medium">Column</th>
							<th class="px-3 py-2 font-medium">Maps to</th>
							<th class="px-3 py-2 font-medium">Sample</th>
						</tr>
					</thead>
					<tbody>
						{#each parsed.headers as header (header)}
							<tr class="border-b border-gray-100 last:border-0">
								<td class="px-3 py-2 font-medium">{header}</td>
								<td class="px-3 py-2">
									<select
										bind:value={fieldByHeader[header]}
										class="rounded border border-gray-300 px-2 py-1 text-sm"
									>
										{#each FIELD_OPTIONS as opt (opt)}
											<option value={opt}>{opt}</option>
										{/each}
									</select>
								</td>
								<td class="px-3 py-2 text-gray-600">
									{parsed.sample[0]?.[header] ?? '—'}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			{#if parsed.suggestion.priceCols.length > 0}
				<div>
					<h2 class="mb-2 text-base font-semibold">Price columns → rate tiers</h2>
					<div class="overflow-hidden rounded border border-gray-200 bg-white">
						<table class="w-full text-sm">
							<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
								<tr>
									<th class="px-3 py-2 font-medium">Column</th>
									<th class="px-3 py-2 font-medium">Tier</th>
									<th class="px-3 py-2 font-medium">New tier name</th>
								</tr>
							</thead>
							<tbody>
								{#each parsed.suggestion.priceCols as pc (pc.header)}
									<tr class="border-b border-gray-100 last:border-0">
										<td class="px-3 py-2 font-medium">{pc.header}</td>
										<td class="px-3 py-2">
											<select
												bind:value={tierChoiceByHeader[pc.header]}
												class="rounded border border-gray-300 px-2 py-1 text-sm"
											>
												{#each rateTiers.items as t (t.id)}
													<option value={String(t.id)}>{t.name}</option>
												{/each}
												<option value={NEW_TIER}>Create new tier…</option>
												<option value={IGNORE_TIER}>Ignore</option>
											</select>
										</td>
										<td class="px-3 py-2">
											{#if tierChoiceByHeader[pc.header] === NEW_TIER}
												<input
													type="text"
													bind:value={newTierNameByHeader[pc.header]}
													class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
												/>
											{:else}
												<span class="text-gray-400">—</span>
											{/if}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			{/if}

			{#if !hasName}
				<p class="text-sm text-amber-700">Map at least one column to "name" to continue.</p>
			{/if}

			<div class="flex gap-2">
				<button
					type="button"
					onclick={preview}
					disabled={previewBusy || !hasName}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{previewBusy ? 'Previewing…' : 'Preview'}
				</button>
				<button
					type="button"
					onclick={backToUpload}
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
				>
					Back
				</button>
			</div>
		</section>
	{/if}

	{#if step === 'diff' && diff}
		<section class="space-y-6">
			<dl class="grid max-w-xl grid-cols-5 gap-2 text-center text-sm">
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

			<p class="text-sm text-gray-500">
				New items (showing first {Math.min(diff.new.length, 10)} of {diff.new.length}); updated
				items: {diff.updated.length}.
			</p>

			<div class="overflow-hidden rounded border border-gray-200 bg-white">
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
								<td colspan="3" class="px-3 py-4 text-center text-gray-500">No new items.</td>
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
						onclick={backToReview}
						class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
					>
						Back
					</button>
				</div>
			</div>
		</section>
	{/if}
</div>
