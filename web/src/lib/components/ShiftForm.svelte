<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import * as shiftsApi from '$lib/api/shifts';
	import { hoursBetween, todayISO } from '$lib/shifts/format';
	import type { Shift, ShiftInput, ShiftMeasure, ShiftStatus } from '$lib/api/types';

	type Props = {
		open: boolean;
		/** Editing target — null for a fresh shift. */
		shift?: Shift | null;
		/** Pre-filled date for a fresh shift (e.g. clicked calendar day). */
		presetDate?: string;
		/** Pre-selected participant for a fresh shift. */
		presetParticipantId?: number | null;
		/** Recording a scheduled shift — adjusts the title + advances status. */
		recording?: boolean;
		/** Called after a successful save (create/update). */
		onsaved?: () => void;
	};

	let {
		open = $bindable(),
		shift = null,
		presetDate = '',
		presetParticipantId = null,
		recording = false,
		onsaved
	}: Props = $props();

	const editing = $derived(shift !== null);
	const title = $derived(recording ? 'Record shift' : editing ? 'Edit shift' : 'Ad-hoc shift');
	const saveLabel = $derived(recording ? 'Save recording' : editing ? 'Save' : 'Add');

	// Form fields. Re-seeded whenever the modal opens (the form is reused).
	let fParticipantId = $state('');
	let fDate = $state('');
	let fStart = $state('');
	let fEnd = $state('');
	let fHours = $state('');
	let fKm = $state('');
	let fNote = $state('');
	let fStatus = $state<ShiftStatus>('recorded');
	let extraMeasures = $state<ShiftMeasure[]>([]);
	let saving = $state(false);
	let error = $state<string | null>(null);

	// Seed the form from props each time the modal transitions to open.
	let lastOpen = false;
	$effect(() => {
		if (open && !lastOpen) {
			seed();
		}
		lastOpen = open;
	});

	function seed(): void {
		error = null;
		extraMeasures = [];
		if (shift) {
			fParticipantId = String(shift.participantId);
			fDate = shift.serviceDate ? shift.serviceDate.slice(0, 10) : todayISO();
			fStart = shift.startTime;
			fEnd = shift.endTime;
			fHours = shift.hours ? String(shift.hours) : '';
			fKm = shift.km ? String(shift.km) : '';
			fNote = shift.note;
			fStatus = shift.status;
			// Preserve any non-km/non-distance measures so an edit doesn't drop them.
			extraMeasures = shift.measures.filter((m) => m.unit !== 'km');
		} else {
			const first = presetParticipantId ?? participants.items[0]?.id ?? null;
			fParticipantId = first === null ? '' : String(first);
			fDate = presetDate || todayISO();
			fStart = '';
			fEnd = '';
			fHours = '';
			fKm = '';
			fNote = '';
			fStatus = 'recorded';
		}
	}

	function onTimeInput(): void {
		if (fStart && fEnd) {
			fHours = String(hoursBetween(fStart, fEnd));
		}
	}

	function addMeasure(): void {
		extraMeasures = [...extraMeasures, { label: '', value: 0, unit: '', code: '' }];
	}

	function removeMeasure(i: number): void {
		extraMeasures = extraMeasures.filter((_, idx) => idx !== i);
	}

	function buildMeasures(): ShiftMeasure[] {
		const out: ShiftMeasure[] = [];
		for (let i = 0; i < extraMeasures.length; i++) {
			const m = extraMeasures[i];
			if (m.label.trim().length > 0) {
				out.push({ label: m.label, value: Number(m.value) || 0, unit: m.unit, code: m.code });
			}
		}
		return out;
	}

	async function submit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		if (fParticipantId === '') {
			error = 'Please select a participant.';
			return;
		}
		if (fDate === '') {
			error = 'Please pick a date.';
			return;
		}
		// Recording a scheduled shift advances it to recorded.
		const nextStatus: ShiftStatus =
			recording && fStatus === 'scheduled' ? 'recorded' : fStatus;
		const input: ShiftInput = {
			participantId: Number(fParticipantId),
			serviceDate: fDate,
			startTime: fStart,
			endTime: fEnd,
			hours: Number(fHours) || 0,
			km: Number(fKm) || 0,
			measures: buildMeasures(),
			note: fNote,
			tags: shift?.tags ?? [],
			status: nextStatus
		};
		saving = true;
		try {
			if (shift) {
				await shiftsApi.update(shift.id, input);
			} else {
				await shiftsApi.create(input);
			}
			open = false;
			onsaved?.();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save shift.';
		} finally {
			saving = false;
		}
	}
</script>

<Modal bind:open {title}>
	<form class="space-y-3" onsubmit={submit}>
		<p class="text-xs text-gray-500">
			Semi-structured: time &amp; participant are structured; note is free text; measures map to
			catalogue codes.
		</p>

		<div class="grid grid-cols-2 gap-3">
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Date</span>
				<input
					type="date"
					bind:value={fDate}
					required
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Participant</span>
				<select
					bind:value={fParticipantId}
					required
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="">— select —</option>
					{#each participants.items as p (p.id)}
						<option value={String(p.id)}>{p.name}</option>
					{/each}
				</select>
			</label>
		</div>

		<div class="grid grid-cols-3 gap-3">
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Start</span>
				<input
					type="time"
					bind:value={fStart}
					oninput={onTimeInput}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="block">
				<span class="mb-1 block text-sm font-medium">End</span>
				<input
					type="time"
					bind:value={fEnd}
					oninput={onTimeInput}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Hours</span>
				<input
					type="number"
					step="0.25"
					min="0"
					bind:value={fHours}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
		</div>

		<div>
			<span class="mb-1 block text-sm font-medium">Measures</span>
			<div class="flex items-center gap-2">
				<span class="w-24 text-sm text-gray-500">Distance</span>
				<input
					type="number"
					min="0"
					bind:value={fKm}
					class="w-28 rounded border border-gray-300 px-2 py-1 text-sm"
				/>
				<span class="text-sm text-gray-500">km → Transport</span>
			</div>
			{#each extraMeasures as m, i (i)}
				<div class="mt-2 flex items-center gap-2">
					<input
						placeholder="measure name"
						bind:value={m.label}
						class="flex-1 rounded border border-gray-300 px-2 py-1 text-sm"
					/>
					<input
						type="number"
						placeholder="value"
						bind:value={m.value}
						class="w-24 rounded border border-gray-300 px-2 py-1 text-sm"
					/>
					<input
						placeholder="unit"
						bind:value={m.unit}
						class="w-20 rounded border border-gray-300 px-2 py-1 text-sm"
					/>
					<button
						type="button"
						onclick={() => removeMeasure(i)}
						aria-label="Remove measure"
						class="text-red-600 hover:underline"
					>
						✕
					</button>
				</div>
			{/each}
			<button
				type="button"
				onclick={addMeasure}
				class="mt-2 rounded border border-gray-300 px-3 py-1 text-sm hover:bg-gray-50"
			>
				+ add measure
			</button>
		</div>

		<label class="block">
			<span class="mb-1 block text-sm font-medium">Note</span>
			<textarea
				bind:value={fNote}
				rows="3"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			></textarea>
		</label>

		{#if error}
			<p class="text-sm text-red-600">{error}</p>
		{/if}

		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={() => (open = false)}
				class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
			>
				Cancel
			</button>
			<button
				type="submit"
				disabled={saving}
				class="rounded bg-green-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{saving ? 'Saving…' : saveLabel}
			</button>
		</div>
	</form>
</Modal>
