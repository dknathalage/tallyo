<script lang="ts" generics="T extends { id: number }, TInput">
	import type { Snippet } from 'svelte';
	import { onDestroy } from 'svelte';
	import { replaceState } from '$app/navigation';
	import type { Column, EditInput } from './datatable';
	import { createAutosave, type SaveState } from './autosave';

	type Crud = {
		get: (id: number) => Promise<T>;
		create: (input: TInput) => Promise<T>;
		update: (id: number, input: TInput) => Promise<T>;
	};

	type Props = {
		title: string;
		columns: Column<T>[];
		crud: Crud;
		/** Existing record id, or 'new' to create. */
		id: number | 'new';
		/** Map the editable draft back to the API input shape. */
		toInput: (draft: T) => TInput;
		/** List route the back-link returns to (e.g. '/tax-rates'). */
		backHref: string;
		/** Starting values for a 'new' record; merged over inferred defaults. */
		blank?: Partial<T>;
		/** Optional per-field validation. Return an error string, or null when valid. */
		validate?: (key: string, value: unknown, draft: T) => string | null;
		/** Entity-specific sections rendered below the fields (rich entities). */
		extras?: Snippet<[T]>;
	};

	let { title, columns, crud, id, toInput, backHref, blank, validate, extras }: Props = $props();

	function inputKind(col: Column<T>): EditInput {
		if (col.input) return col.input;
		if (col.filter === 'number') return 'number';
		if (col.filter === 'date') return 'date';
		if (col.filter === 'enum') return 'select';
		return 'text';
	}

	function defaultFor(col: Column<T>): unknown {
		switch (inputKind(col)) {
			case 'number':
				return 0;
			case 'checkbox':
				return false;
			case 'select':
				return col.values?.[0] ?? '';
			default:
				return '';
		}
	}

	// ── Load / seed draft ──────────────────────────────────────────────────────
	let draft = $state<T | null>(null);
	let loadError = $state<string | null>(null);
	// svelte-ignore state_referenced_locally -- recordId seeds from the prop once; onCreated then owns it independently.
	let recordId = $state<number | 'new'>(id);

	async function init(): Promise<void> {
		const currentId = id;
		if (currentId === 'new') {
			const seed: Record<string, unknown> = { id: 0 };
			for (const col of columns) seed[col.key] = defaultFor(col);
			draft = { ...(seed as T), ...(blank ?? {}) };
			return;
		}
		try {
			draft = await crud.get(currentId);
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to load record.';
		}
	}
	void init();

	// ── Autosave wiring ─────────────────────────────────────────────────────────
	let saveState = $state<SaveState>('idle');
	const autosave = createAutosave<TInput, T>({
		create: (input) => crud.create(input),
		update: (existingId, input) => crud.update(existingId, input),
		onState: (s) => (saveState = s),
		onCreated: (newId) => {
			recordId = newId;
			replaceState(`${backHref}/${newId}`, {});
		}
	});
	onDestroy(() => autosave.dispose());

	let errors = $state<Record<string, string | null>>({});

	function edit(col: Column<T>, value: unknown): void {
		if (!draft) return;
		draft = { ...draft, [col.key]: value };
		const msg = validate ? validate(col.key, value, draft) : null;
		errors = { ...errors, [col.key]: msg };
		// Withhold the whole save while any field is invalid.
		if (Object.values(errors).some((e) => e)) return;
		autosave.schedule(toInput(draft));
	}

	function num(v: string): number {
		const n = Number(v);
		return Number.isFinite(n) ? n : 0;
	}

	// Manual save — a safety net beside the debounced autosave. Forces the current
	// (valid) draft to persist immediately rather than waiting for the debounce.
	function saveNow(): void {
		if (!draft) return;
		if (Object.values(errors).some((e) => e)) return;
		autosave.schedule(toInput(draft));
		autosave.flush();
	}
</script>

<div class="space-y-5">
	<div class="flex items-center justify-between">
		<a href={backHref} class="text-sm text-gray-500 hover:text-gray-900">← Back</a>
		<div class="flex items-center gap-3">
			<span class="h-4 text-xs">
				{#if saveState === 'saving'}<span class="text-gray-400">saving…</span>
				{:else if saveState === 'saved'}<span class="text-green-600">✓ saved</span>
				{:else if saveState === 'error'}
					<span class="text-red-600"
						>⚠ error ·
						<button type="button" class="underline" onclick={() => autosave.retry()}>retry</button>
					</span>
				{/if}
			</span>
			<button
				type="button"
				onclick={saveNow}
				disabled={!draft || saveState === 'saving'}
				class="rounded bg-gray-900 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
			>
				Save
			</button>
		</div>
	</div>

	<h1 class="text-xl font-semibold">{recordId === 'new' ? `New ${title}` : title}</h1>

	{#if loadError}
		<p class="text-sm text-red-600">{loadError}</p>
	{:else if !draft}
		<p class="text-sm text-gray-500">Loading…</p>
	{:else}
		<div class="max-w-xl space-y-4">
			{#each columns as col (col.key)}
				{@const kind = inputKind(col)}
				<label class="block">
					<span class="mb-1 block text-sm font-medium">{col.label}</span>
					{#if kind === 'readonly'}
						<p class="text-sm text-gray-600">{col.cell ? col.cell(draft) : String((draft as Record<string, unknown>)[col.key] ?? '')}</p>
					{:else if kind === 'checkbox'}
						<input
							type="checkbox"
							checked={Boolean((draft as Record<string, unknown>)[col.key])}
							onchange={(e) => edit(col, e.currentTarget.checked)}
							class="h-4 w-4"
						/>
					{:else if kind === 'select'}
						<select
							value={String((draft as Record<string, unknown>)[col.key] ?? '')}
							onchange={(e) => edit(col, e.currentTarget.value)}
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						>
							{#each col.values ?? [] as opt (opt)}<option value={opt}>{opt}</option>{/each}
						</select>
					{:else if kind === 'textarea'}
						<textarea
							value={String((draft as Record<string, unknown>)[col.key] ?? '')}
							oninput={(e) => edit(col, e.currentTarget.value)}
							rows="3"
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						></textarea>
					{:else}
						<input
							type={kind === 'number' ? 'number' : kind === 'date' ? 'date' : 'text'}
							value={String((draft as Record<string, unknown>)[col.key] ?? '')}
							oninput={(e) =>
								edit(col, kind === 'number' ? num(e.currentTarget.value) : e.currentTarget.value)}
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						/>
					{/if}
					{#if errors[col.key]}<span class="mt-1 block text-xs text-red-600">{errors[col.key]}</span>{/if}
				</label>
			{/each}
		</div>

		{#if extras}{@render extras(draft)}{/if}
	{/if}
</div>
