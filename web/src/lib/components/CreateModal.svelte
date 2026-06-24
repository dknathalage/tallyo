<script lang="ts" generics="T extends { id: string }, TInput">
	import type { Column, EditInput } from './datatable';
	import Modal from './Modal.svelte';
	import Button from './Button.svelte';

	type Props = {
		title: string;
		columns: Column<T>[];
		/** Create the record from the assembled input. */
		create: (input: TInput) => Promise<T>;
		/** Map the editable draft to the API input shape. */
		toInput: (draft: T) => TInput;
		/** Starting values, merged over inferred defaults. */
		blank?: Partial<T>;
		/** Optional per-field validation. Return an error string, or null when valid. */
		validate?: (key: string, value: unknown, draft: T) => string | null;
		open?: boolean;
		/** Called after a successful create. */
		onsaved?: (created: T) => void;
	};

	let { title, columns, create, toInput, blank, validate, open = $bindable(false), onsaved }: Props =
		$props();

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

	let draft = $state<T | null>(null);
	let errors = $state<Record<string, string | null>>({});
	let saving = $state(false);
	let saveError = $state<string | null>(null);

	// Modal mounts its body lazily, so seed BEFORE that render (see docs/gotchas.md):
	// re-seed each time the modal transitions to open (the component is reused).
	let lastOpen = false;
	$effect.pre(() => {
		if (open && !lastOpen) seed();
		lastOpen = open;
	});

	function seed(): void {
		errors = {};
		saveError = null;
		const base: Record<string, unknown> = { id: '' };
		for (const col of columns) base[col.key] = defaultFor(col);
		draft = { ...(base as T), ...(blank ?? {}) };
	}

	function num(v: string): number {
		const n = Number(v);
		return Number.isFinite(n) ? n : 0;
	}

	function edit(col: Column<T>, value: unknown): void {
		if (!draft) return;
		draft = { ...draft, [col.key]: value };
		errors = { ...errors, [col.key]: validate ? validate(col.key, value, draft) : null };
	}

	async function save(): Promise<void> {
		if (!draft) return;
		// Validate every field up front — a create can't lean on per-keystroke checks.
		const next: Record<string, string | null> = {};
		for (const col of columns) {
			next[col.key] = validate ? validate(col.key, (draft as Record<string, unknown>)[col.key], draft) : null;
		}
		errors = next;
		if (Object.values(next).some((e) => e)) return;
		saving = true;
		saveError = null;
		try {
			const created = await create(toInput(draft));
			open = false;
			onsaved?.(created);
		} catch (err) {
			saveError = err instanceof Error ? err.message : 'Failed to create.';
		} finally {
			saving = false;
		}
	}
</script>

<Modal bind:open title={`New ${title}`}>
	{#if draft}
		{#if saveError}<p class="mb-3 text-sm text-red-600">{saveError}</p>{/if}
		<div class="space-y-4">
			{#each columns as col (col.key)}
				{@const kind = inputKind(col)}
				{#if kind !== 'readonly'}
					<label class="block">
						<span class="mb-1 block text-sm font-medium">{col.label}</span>
						{#if kind === 'checkbox'}
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
								class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
							>
								{#each col.values ?? [] as opt (opt)}<option value={opt}>{opt}</option>{/each}
							</select>
						{:else if kind === 'textarea'}
							<textarea
								value={String((draft as Record<string, unknown>)[col.key] ?? '')}
								oninput={(e) => edit(col, e.currentTarget.value)}
								rows="3"
								class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
							></textarea>
						{:else}
							<input
								type={kind === 'number' ? 'number' : kind === 'date' ? 'date' : 'text'}
								value={String((draft as Record<string, unknown>)[col.key] ?? '')}
								oninput={(e) =>
									edit(col, kind === 'number' ? num(e.currentTarget.value) : e.currentTarget.value)}
								class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm {kind ===
								'number'
									? 'font-mono tabular-nums'
									: ''}"
							/>
						{/if}
						{#if errors[col.key]}<span class="mt-1 block text-xs text-red-600">{errors[col.key]}</span>{/if}
					</label>
				{/if}
			{/each}
		</div>
	{/if}

	{#snippet footer()}
		<Button variant="ghost" onclick={() => (open = false)} disabled={saving}>Cancel</Button>
		<Button onclick={save} loading={saving} disabled={saving}>Create</Button>
	{/snippet}
</Modal>
