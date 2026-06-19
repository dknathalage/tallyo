<script lang="ts">
	import { onMount } from 'svelte';
	import { shifts } from '$lib/stores/shifts.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import * as shiftsApi from '$lib/api/shifts';
	import ShiftTable from '$lib/components/ShiftTable.svelte';
	import ShiftForm from '$lib/components/ShiftForm.svelte';
	import InvoiceSuggestions from '$lib/components/InvoiceSuggestions.svelte';
	import { shortDate, statusLabel, todayISO } from '$lib/shifts/format';
	import type { Shift, ShiftStatus } from '$lib/api/types';

	const STAGES: ShiftStatus[] = ['scheduled', 'recorded', 'drafted', 'sent', 'paid'];

	onMount(() => {
		shifts.ensureSubscribed();
		void shifts.load();
		participants.ensureSubscribed();
		void participants.load();
	});

	function participantName(id: number): string {
		const p = participants.items.find((x) => x.id === id);
		return p ? p.name : `#${id}`;
	}

	// Pipeline counts per status.
	function stageCount(status: ShiftStatus): number {
		let n = 0;
		for (let i = 0; i < shifts.items.length; i++) {
			if (shifts.items[i].status === status) n++;
		}
		return n;
	}

	const today = todayISO();
	function whenLabel(date: string): string {
		if (date < today) return '⚠ Overdue';
		if (date === today) return 'Today';
		return 'Upcoming';
	}

	// ---- Shift form (record / edit / ad-hoc) ----
	let formOpen = $state(false);
	let formShift = $state<Shift | null>(null);
	let formRecording = $state(false);

	function openAdHoc(): void {
		formShift = null;
		formRecording = false;
		formOpen = true;
	}

	function openRecord(shift: Shift): void {
		formShift = shift;
		formRecording = shift.status === 'scheduled';
		formOpen = true;
	}

	function onShiftSaved(): void {
		void shifts.load();
	}

	async function deleteShifts(ids: number[]): Promise<void> {
		for (const id of ids) {
			await shiftsApi.remove(id);
		}
		await shifts.load();
	}

	// ---- Quick add (paste timesheet → import) ----
	let importParticipant = $state('');
	let importText = $state('');
	let importing = $state(false);
	let importError = $state<string | null>(null);
	let importMsg = $state<string | null>(null);

	async function runImport(): Promise<void> {
		importError = null;
		importMsg = null;
		if (importParticipant === '') {
			importError = 'Select a participant to import shifts for.';
			return;
		}
		if (importText.trim().length === 0) {
			importError = 'Paste a timesheet to extract shifts.';
			return;
		}
		importing = true;
		try {
			const created = await shiftsApi.importShifts(Number(importParticipant), importText);
			importText = '';
			importMsg = `${created.length} shift${created.length === 1 ? '' : 's'} extracted · recorded`;
			await shifts.load();
		} catch (err) {
			importError = err instanceof Error ? err.message : 'Failed to extract shifts.';
		} finally {
			importing = false;
		}
	}

</script>

<div class="space-y-6">
	<div class="flex items-start justify-between gap-4">
		<div>
			<h1 class="mb-1 text-xl font-semibold">Shifts</h1>
			<p class="text-sm text-gray-500">
				Record scheduled shifts, then draft invoices from recorded work.
			</p>
		</div>
		<button
			type="button"
			onclick={openAdHoc}
			class="shrink-0 rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
		>
			+ Ad-hoc shift
		</button>
	</div>

	{#if shifts.error}
		<p class="text-sm text-red-600">{shifts.error}</p>
	{/if}

	<!-- Pipeline strip -->
	<div class="grid grid-cols-2 gap-3 sm:grid-cols-5">
		{#each STAGES as stage (stage)}
			<div class="rounded-lg border border-gray-200 bg-white px-3 py-3 text-center">
				<span class="block text-2xl font-bold">{stageCount(stage)}</span>
				<span class="text-xs tracking-wide text-gray-500 uppercase">{statusLabel(stage)}</span>
			</div>
		{/each}
	</div>

	<!-- Shifts to record -->
	{#if shifts.toRecord.length > 0}
		<section class="rounded-lg border border-amber-200 bg-white p-4" aria-label="Shifts to record">
			<h2 class="mb-3 text-xs font-semibold tracking-wide text-amber-700 uppercase">
				⏱ Shifts to record ({shifts.toRecord.length})
			</h2>
			<div class="space-y-2">
				{#each shifts.toRecord as s (s.id)}
					<div class="flex flex-wrap items-center gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2">
						<span class="min-w-[8rem] font-semibold">{whenLabel(s.serviceDate)} · {shortDate(s.serviceDate)}</span>
						<span class="flex-1 text-sm">
							{participantName(s.participantId)}
							<span class="text-gray-500">· scheduled {s.startTime}–{s.endTime}</span>
						</span>
						<button
							type="button"
							onclick={() => openRecord(s)}
							class="rounded bg-amber-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-amber-700"
						>
							Record shift →
						</button>
					</div>
				{/each}
			</div>
			<p class="mt-2 text-xs text-gray-500">
				Tallyo asks you to record each scheduled shift — add a note, hours/time, distance and other
				measures.
			</p>
		</section>
	{/if}

	<!-- Quick add (paste timesheet → import) -->
	<section class="rounded-lg border border-gray-200 bg-white p-4">
		<h2 class="mb-2 text-xs font-semibold tracking-wide text-gray-500 uppercase">Quick add</h2>
		<div class="mb-2 max-w-xs">
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Participant</span>
				<select
					bind:value={importParticipant}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="">— select —</option>
					{#each participants.items as p (p.id)}
						<option value={String(p.id)}>{p.name}</option>
					{/each}
				</select>
			</label>
		</div>
		<textarea
			bind:value={importText}
			rows="3"
			placeholder="Paste a worker's timesheet — AI extracts shifts, fills time/note, auto-tags catalogue codes."
			class="w-full rounded border border-gray-300 px-3 py-2 font-mono text-sm"
		></textarea>
		<div class="mt-2 flex flex-wrap items-center gap-3">
			<button
				type="button"
				onclick={runImport}
				disabled={importing}
				class="rounded bg-gray-900 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
			>
				{importing ? 'Extracting…' : 'Extract shifts'}
			</button>
			<button
				type="button"
				onclick={openAdHoc}
				class="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
			>
				+ Ad-hoc shift
			</button>
			<span class="text-sm text-gray-500">…or click a Calendar day.</span>
		</div>
		{#if importError}
			<p class="mt-2 text-sm text-red-600">{importError}</p>
		{/if}
		{#if importMsg}
			<p class="mt-2 text-sm text-green-700">{importMsg}</p>
		{/if}
	</section>

	<!-- AI invoice suggestions -->
	<InvoiceSuggestions suggestions={shifts.suggestions} nameFor={participantName} />

	<!-- Shift table -->
	<section>
		{#if shifts.loading && shifts.items.length === 0}
			<p class="text-sm text-gray-500">Loading…</p>
		{:else}
			<ShiftTable shifts={shifts.items} {participantName} onopen={openRecord} ondelete={deleteShifts} />
		{/if}
		<p class="mt-2 text-xs text-gray-500">
			Status pipeline: scheduled → recorded → drafted → sent → paid.
		</p>
	</section>
</div>

<ShiftForm
	bind:open={formOpen}
	shift={formShift}
	recording={formRecording}
	onsaved={onShiftSaved}
/>
