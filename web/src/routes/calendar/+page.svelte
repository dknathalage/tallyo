<script lang="ts">
	import { onMount } from 'svelte';
	import { shifts } from '$lib/stores/shifts.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import Calendar from '$lib/components/Calendar.svelte';
	import ShiftForm from '$lib/components/ShiftForm.svelte';
	import { todayISO } from '$lib/shifts/format';
	import type { Shift } from '$lib/api/types';

	const MONTH_NAMES = [
		'January',
		'February',
		'March',
		'April',
		'May',
		'June',
		'July',
		'August',
		'September',
		'October',
		'November',
		'December'
	];

	const today = todayISO();

	// Current view month — seeded from today.
	let viewYear = $state(Number(today.slice(0, 4)));
	let viewMonth = $state(Number(today.slice(5, 7)) - 1); // 0-based

	const month = $derived(`${viewYear}-${String(viewMonth + 1).padStart(2, '0')}`);

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

	function prevMonth(): void {
		if (viewMonth === 0) {
			viewMonth = 11;
			viewYear -= 1;
		} else {
			viewMonth -= 1;
		}
	}

	function nextMonth(): void {
		if (viewMonth === 11) {
			viewMonth = 0;
			viewYear += 1;
		} else {
			viewMonth += 1;
		}
	}

	function thisMonth(): void {
		viewYear = Number(today.slice(0, 4));
		viewMonth = Number(today.slice(5, 7)) - 1;
	}

	// ---- Shift form ----
	let formOpen = $state(false);
	let formShift = $state<Shift | null>(null);
	let formRecording = $state(false);
	let formDate = $state('');

	function openDay(dateISO: string): void {
		formShift = null;
		formRecording = false;
		formDate = dateISO;
		formOpen = true;
	}

	function openShift(s: Shift): void {
		formShift = s;
		formRecording = s.status === 'scheduled';
		formDate = '';
		formOpen = true;
	}

	function onSaved(): void {
		void shifts.load();
	}
</script>

<div class="space-y-6">
	<div class="flex flex-wrap items-center justify-between gap-3">
		<h1 class="text-xl font-semibold">Calendar — {MONTH_NAMES[viewMonth]} {viewYear}</h1>
		<div class="flex items-center gap-2">
			<button
				type="button"
				onclick={prevMonth}
				class="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
				aria-label="Previous month">←</button
			>
			<button
				type="button"
				onclick={thisMonth}
				class="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50">Today</button
			>
			<button
				type="button"
				onclick={nextMonth}
				class="rounded border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-50"
				aria-label="Next month">→</button
			>
		</div>
	</div>

	{#if shifts.error}
		<p class="text-sm text-red-600">{shifts.error}</p>
	{/if}

	<div class="rounded-lg border border-gray-200 bg-white p-3">
		<Calendar
			shifts={shifts.items}
			nameFor={participantName}
			{month}
			onaddday={openDay}
			onopen={openShift}
		/>
	</div>

	<p class="text-sm text-gray-500">
		Amber dashed = scheduled (click to record) · blue recorded · violet drafted · indigo sent ·
		green paid. Click any day to add an ad-hoc shift.
	</p>
</div>

<ShiftForm
	bind:open={formOpen}
	shift={formShift}
	recording={formRecording}
	presetDate={formDate}
	onsaved={onSaved}
/>
