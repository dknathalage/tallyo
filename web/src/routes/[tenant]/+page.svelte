<script lang="ts">
	import { onMount } from 'svelte';
	import { shifts } from '$lib/stores/shifts.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import * as shiftsApi from '$lib/api/shifts';
	import ShiftTable from '$lib/components/ShiftTable.svelte';
	import ShiftForm from '$lib/components/ShiftForm.svelte';
	import { shortDate, todayISO } from '$lib/shifts/format';
	import type { Shift } from '$lib/api/types';

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

	const today = todayISO();
	function whenLabel(date: string): string {
		if (date < today) return '⚠ Overdue';
		if (date === today) return 'Today';
		return 'Upcoming';
	}

	// ---- Shift form (record / edit / add) ----
	let formOpen = $state(false);
	let formShift = $state<Shift | null>(null);
	let formRecording = $state(false);
	let formDate = $state('');

	function openNew(): void {
		formShift = null;
		formRecording = false;
		formDate = '';
		formOpen = true;
	}

	function openRecord(shift: Shift): void {
		formShift = shift;
		formRecording = shift.status === 'scheduled';
		formDate = '';
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
			onclick={openNew}
			class="shrink-0 rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
		>
			+ Add shift
		</button>
	</div>

	{#if shifts.error}
		<p class="text-sm text-red-600">{shifts.error}</p>
	{/if}

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
							<span class="text-gray-500">· scheduled</span>
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


	{#if shifts.loading && shifts.items.length === 0}
		<p class="text-sm text-gray-500">Loading…</p>
	{:else}
		<section>
			<ShiftTable shifts={shifts.items} {participantName} onopen={openRecord} ondelete={deleteShifts} />
			<p class="mt-2 text-xs text-gray-500">
				Status pipeline: scheduled → recorded → drafted → sent → paid.
			</p>
		</section>
	{/if}
</div>

<ShiftForm
	bind:open={formOpen}
	shift={formShift}
	recording={formRecording}
	presetDate={formDate}
	onsaved={onShiftSaved}
/>
