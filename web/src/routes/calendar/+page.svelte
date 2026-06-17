<script lang="ts">
	import { onMount } from 'svelte';
	import { shifts } from '$lib/stores/shifts.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import ShiftForm from '$lib/components/ShiftForm.svelte';
	import { eventClass, todayISO } from '$lib/shifts/format';
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
	const DOW = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

	const today = todayISO();

	// Current view month — seeded from today.
	let viewYear = $state(Number(today.slice(0, 4)));
	let viewMonth = $state(Number(today.slice(5, 7)) - 1); // 0-based

	onMount(() => {
		shifts.ensureSubscribed();
		void shifts.load();
		participants.ensureSubscribed();
		void participants.load();
	});

	function firstName(id: number): string {
		const p = participants.items.find((x) => x.id === id);
		if (!p) return `#${id}`;
		return p.name.split(' ')[0];
	}

	function pad(n: number): string {
		return String(n).padStart(2, '0');
	}

	// ISO date for a day-of-month in the current view.
	function isoFor(day: number): string {
		return `${viewYear}-${pad(viewMonth + 1)}-${pad(day)}`;
	}

	const daysInMonth = $derived(new Date(viewYear, viewMonth + 1, 0).getDate());

	// Leading blanks so the 1st lands under the right weekday (Mon-first grid).
	const leadingBlanks = $derived.by<number[]>(() => {
		const jsDow = new Date(viewYear, viewMonth, 1).getDay(); // 0=Sun
		const monFirst = (jsDow + 6) % 7; // 0=Mon
		return Array.from({ length: monFirst }, (_, i) => i);
	});

	// Shifts grouped by their day-of-month within the current view month.
	const byDay = $derived.by<Map<number, Shift[]>>(() => {
		const map = new Map<number, Shift[]>();
		const prefix = `${viewYear}-${pad(viewMonth + 1)}-`;
		for (let i = 0; i < shifts.items.length; i++) {
			const s = shifts.items[i];
			if (!s.serviceDate.startsWith(prefix)) continue;
			const day = Number(s.serviceDate.slice(8, 10));
			const list = map.get(day) ?? [];
			list.push(s);
			map.set(day, list);
		}
		return map;
	});

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

	// ---- Shift form ----
	let formOpen = $state(false);
	let formShift = $state<Shift | null>(null);
	let formRecording = $state(false);
	let formDate = $state('');

	function openDay(day: number): void {
		formShift = null;
		formRecording = false;
		formDate = isoFor(day);
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
		<div class="grid grid-cols-7 gap-1.5">
			{#each DOW as d (d)}
				<div class="text-center text-xs font-semibold text-gray-500">{d}</div>
			{/each}
			{#each leadingBlanks as b (b)}
				<div></div>
			{/each}
			{#each Array.from({ length: daysInMonth }, (_, i) => i + 1) as day (day)}
				{@const iso = isoFor(day)}
				{@const isToday = iso === today}
				{@const dayShifts = byDay.get(day) ?? []}
				<button
					type="button"
					onclick={() => openDay(day)}
					class="min-h-[4.5rem] rounded-lg border p-1.5 text-left align-top hover:bg-gray-50 {isToday
						? 'border-blue-500 ring-1 ring-blue-500'
						: 'border-gray-200'}"
				>
					<div class="text-xs text-gray-500">{day}{isToday ? ' • today' : ''}</div>
					<div class="mt-1 space-y-1">
						{#each dayShifts as s (s.id)}
							<span
								role="button"
								tabindex="0"
								onclick={(e) => {
									e.stopPropagation();
									openShift(s);
								}}
								onkeydown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.stopPropagation();
										openShift(s);
									}
								}}
								class="block truncate rounded px-1 py-0.5 text-[10.5px] font-semibold {eventClass(
									s.status
								)}"
							>
								{firstName(s.participantId)}
								{s.hours ? `${s.hours}h` : s.startTime}
							</span>
						{/each}
					</div>
				</button>
			{/each}
		</div>
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
