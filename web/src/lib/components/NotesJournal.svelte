<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { createNotesStore } from '$lib/stores/notes.svelte';
	import { agentChat } from '$lib/stores/agentChat.svelte';
	import * as notesApi from '$lib/api/notes';
	import {
		resolvePreset,
		toISODate,
		PRESET_LABELS,
		type RangePreset,
		type DateRange
	} from '$lib/dateRange';
	import type { Note } from '$lib/api/types';

	type Props = { participantId: number; participantName: string };
	const { participantId, participantName }: Props = $props();

	const store = createNotesStore();

	// ---- Date-range filter (drives both the listing and the invoice action) ----
	let preset = $state<RangePreset>('last-30');
	let customFrom = $state('');
	let customTo = $state('');

	// The resolved absolute range for the current preset/custom selection. Returns
	// null when a custom range is incomplete (so we can disable dependent actions).
	const range = $derived.by<DateRange | null>(() => {
		if (preset === 'custom') {
			if (customFrom === '' || customTo === '') return null;
			return { from: customFrom, to: customTo };
		}
		return resolvePreset(preset);
	});

	// Re-scope the store whenever the resolved range changes.
	$effect(() => {
		const r = range;
		if (r === null) return;
		store.setScope(participantId, r.from, r.to);
	});

	onMount(() => {
		store.ensureSubscribed();
	});

	// ---- Grouping: notes by serviceDate, newest day first ----
	type DayGroup = { date: string; notes: Note[] };
	const groups = $derived.by<DayGroup[]>(() => {
		const byDate = new Map<string, Note[]>();
		// Bounded by the fetched note count.
		for (const n of store.items) {
			const key = n.serviceDate.slice(0, 10);
			const bucket = byDate.get(key);
			if (bucket === undefined) byDate.set(key, [n]);
			else bucket.push(n);
		}
		const out: DayGroup[] = [];
		for (const [date, ns] of byDate) out.push({ date, notes: ns });
		out.sort((a, b) => (a.date < b.date ? 1 : a.date > b.date ? -1 : 0));
		return out;
	});

	// ---- Add / edit form ----
	const today = toISODate(new Date());
	let formServiceDate = $state(today);
	let formBody = $state('');
	let formKm = $state('');
	let formHours = $state('');
	let editId = $state<number | null>(null);
	let saving = $state(false);
	let formError = $state<string | null>(null);

	// Parse an optional non-negative number field; '' → null. Returns undefined to
	// signal an invalid (non-numeric / negative) value the caller must reject.
	function parseOptionalNumber(v: string): number | null | undefined {
		const t = v.trim();
		if (t === '') return null;
		const n = Number(t);
		if (!Number.isFinite(n) || n < 0) return undefined;
		return n;
	}

	function resetForm(): void {
		editId = null;
		formServiceDate = today;
		formBody = '';
		formKm = '';
		formHours = '';
		formError = null;
	}

	function startEdit(n: Note): void {
		editId = n.id;
		formServiceDate = n.serviceDate.slice(0, 10);
		formBody = n.body;
		formKm = n.transportKm === null ? '' : String(n.transportKm);
		formHours = n.supportHours === null ? '' : String(n.supportHours);
		formError = null;
	}

	async function submitForm(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		if (formBody.trim() === '') {
			formError = 'Note body is required.';
			return;
		}
		if (formServiceDate === '') {
			formError = 'Service date is required.';
			return;
		}
		const km = parseOptionalNumber(formKm);
		const hours = parseOptionalNumber(formHours);
		if (km === undefined) {
			formError = 'Transport km must be a non-negative number.';
			return;
		}
		if (hours === undefined) {
			formError = 'Support hours must be a non-negative number.';
			return;
		}
		saving = true;
		try {
			const input = {
				participantId,
				serviceDate: formServiceDate,
				body: formBody.trim(),
				transportKm: km,
				supportHours: hours
			};
			if (editId === null) await notesApi.create(input);
			else await notesApi.update(editId, input);
			resetForm();
			await store.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to save note.';
		} finally {
			saving = false;
		}
	}

	let rowError = $state<string | null>(null);
	let rowBusy = $state(false);

	async function removeNote(id: number): Promise<void> {
		rowError = null;
		rowBusy = true;
		try {
			await notesApi.remove(id);
			if (editId === id) resetForm();
			await store.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete note.';
		} finally {
			rowBusy = false;
		}
	}

	// ---- Create invoice from notes (seed an agent run) ----
	let seeding = $state(false);
	let seedError = $state<string | null>(null);

	async function createInvoiceFromNotes(): Promise<void> {
		seedError = null;
		const r = range;
		if (r === null) {
			seedError = 'Pick a complete date range first.';
			return;
		}
		seeding = true;
		try {
			const prompt =
				`Draft an invoice for ${participantName} for services between ${r.from} and ${r.to} ` +
				`from their notes. List the notes, look up the correct NDIS support item codes and rates, ` +
				`and create the invoice for my approval.`;
			// Start a fresh conversation, surface the chat pane (home route), then send.
			// agentChat.send() lazily creates the conversation when none is active.
			agentChat.newConversation();
			await goto('/');
			await agentChat.send(prompt);
		} catch (err) {
			seedError = err instanceof Error ? err.message : 'Failed to start the invoice draft.';
		} finally {
			seeding = false;
		}
	}
</script>

<div class="space-y-6">
	<!-- Range filter -->
	<div class="flex flex-wrap items-end gap-3">
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Range</span>
			<select bind:value={preset} class="rounded border border-gray-300 px-3 py-2 text-sm">
				{#each Object.keys(PRESET_LABELS) as p (p)}
					<option value={p}>{PRESET_LABELS[p as RangePreset]}</option>
				{/each}
			</select>
		</label>
		{#if preset === 'custom'}
			<label class="block">
				<span class="mb-1 block text-sm font-medium">From</span>
				<input
					type="date"
					bind:value={customFrom}
					class="rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="block">
				<span class="mb-1 block text-sm font-medium">To</span>
				<input
					type="date"
					bind:value={customTo}
					class="rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
		{:else if range}
			<p class="pb-2 text-sm text-gray-500">{range.from} – {range.to}</p>
		{/if}
		<button
			type="button"
			onclick={createInvoiceFromNotes}
			disabled={seeding || range === null}
			class="ml-auto rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
		>
			{seeding ? 'Starting…' : 'Create invoice from notes'}
		</button>
	</div>
	{#if seedError}
		<p class="text-sm text-red-600">{seedError}</p>
	{/if}

	<!-- Add / edit entry form -->
	<form class="grid grid-cols-2 gap-3 rounded border border-gray-200 bg-gray-50 p-4" onsubmit={submitForm}>
		<div class="col-span-2 text-sm font-medium">
			{editId === null ? 'New note' : 'Edit note'}
		</div>
		<label class="col-span-1">
			<span class="mb-1 block text-sm font-medium">Service date</span>
			<input
				type="date"
				bind:value={formServiceDate}
				required
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>
		<div class="col-span-1 grid grid-cols-2 gap-3">
			<label>
				<span class="mb-1 block text-sm font-medium">Transport km</span>
				<input
					type="number"
					min="0"
					step="any"
					bind:value={formKm}
					placeholder="optional"
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label>
				<span class="mb-1 block text-sm font-medium">Support hours</span>
				<input
					type="number"
					min="0"
					step="any"
					bind:value={formHours}
					placeholder="optional"
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
		</div>
		<label class="col-span-2">
			<span class="mb-1 block text-sm font-medium">What did you do?</span>
			<textarea
				bind:value={formBody}
				required
				rows="3"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			></textarea>
		</label>
		{#if formError}
			<p class="col-span-2 text-sm text-red-600">{formError}</p>
		{/if}
		<div class="col-span-2 flex gap-2">
			<button
				type="submit"
				disabled={saving}
				class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{saving ? 'Saving…' : editId === null ? 'Add note' : 'Save note'}
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

	<!-- Listing grouped by day -->
	{#if store.loading}
		<p class="text-sm text-gray-500">Loading…</p>
	{/if}
	{#if store.error}
		<p class="text-sm text-red-600">{store.error}</p>
	{/if}
	{#if rowError}
		<p class="text-sm text-red-600">{rowError}</p>
	{/if}

	<div class="space-y-5">
		{#each groups as g (g.date)}
			<section>
				<h3 class="mb-2 text-sm font-semibold text-gray-700">{g.date}</h3>
				<ul class="space-y-2">
					{#each g.notes as n (n.id)}
						<li class="rounded border border-gray-200 bg-white p-3">
							<div class="flex items-start justify-between gap-3">
								<p class="whitespace-pre-wrap text-sm text-gray-900">{n.body}</p>
								{#if n.billedInvoiceId !== null}
									<span
										class="shrink-0 rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800"
									>
										Billed
									</span>
								{/if}
							</div>
							<div class="mt-2 flex items-center gap-4 text-xs text-gray-500">
								{#if n.transportKm !== null}
									<span>{n.transportKm} km</span>
								{/if}
								{#if n.supportHours !== null}
									<span>{n.supportHours} h</span>
								{/if}
								<span class="ml-auto flex gap-3">
									<button
										type="button"
										onclick={() => startEdit(n)}
										class="text-gray-900 hover:underline"
									>
										Edit
									</button>
									<button
										type="button"
										disabled={rowBusy}
										onclick={() => removeNote(n.id)}
										class="text-red-600 hover:underline disabled:opacity-50"
									>
										Delete
									</button>
								</span>
							</div>
						</li>
					{/each}
				</ul>
			</section>
		{:else}
			<p class="text-sm text-gray-500">No notes in this range.</p>
		{/each}
	</div>
</div>
