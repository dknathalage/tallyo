<script lang="ts">
	import { eventClass, todayISO } from '$lib/shifts/format';
	import type { Shift } from '$lib/api/types';

	type Props = {
		shifts: Shift[];
		/** Resolve a participant id to a display name (chips show the first name). */
		nameFor: (participantId: number) => string;
		/** Month to render, as a YYYY-MM string. Defaults to the current month. */
		month?: string;
		/** Click an empty day cell → add a shift on that date. */
		onaddday?: (dateISO: string) => void;
		/** Click a shift chip → edit (or record when scheduled). */
		onopen?: (shift: Shift) => void;
	};

	let {
		shifts,
		nameFor,
		month = todayISO().slice(0, 7),
		onaddday,
		onopen
	}: Props = $props();

	const DOW = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

	const year = $derived(Number(month.slice(0, 4)));
	const monthIndex = $derived(Number(month.slice(5, 7)) - 1);

	// Days in the rendered month + the Monday-based weekday offset of day 1.
	const daysInMonth = $derived(new Date(year, monthIndex + 1, 0).getDate());
	const leadingBlanks = $derived((new Date(year, monthIndex, 1).getDay() + 6) % 7);

	const today = todayISO();

	function isoFor(day: number): string {
		const m = String(monthIndex + 1).padStart(2, '0');
		const d = String(day).padStart(2, '0');
		return `${year}-${m}-${d}`;
	}

	// Group shifts by their day-of-month within this rendered month.
	const byDay = $derived.by<Map<number, Shift[]>>(() => {
		const map = new Map<number, Shift[]>();
		for (let i = 0; i < shifts.length; i++) {
			const s = shifts[i];
			if (s.serviceDate.slice(0, 7) !== month) continue;
			const day = Number(s.serviceDate.slice(8, 10));
			const list = map.get(day) ?? [];
			list.push(s);
			map.set(day, list);
		}
		return map;
	});

	const days = $derived(Array.from({ length: daysInMonth }, (_, i) => i + 1));
	const blanks = $derived(Array.from({ length: leadingBlanks }, (_, i) => i));

	function firstName(participantId: number): string {
		return nameFor(participantId).split(' ')[0] ?? '';
	}

	function chipLabel(s: Shift): string {
		return `${firstName(s.participantId)} ${s.hours ? `${s.hours}h` : s.startTime || ''}`.trim();
	}
</script>

<div class="grid grid-cols-7 gap-1.5">
	{#each DOW as d (d)}
		<div class="text-center text-xs font-semibold text-gray-500">{d}</div>
	{/each}
	{#each blanks as b (b)}
		<div></div>
	{/each}
	{#each days as day (day)}
		{@const iso = isoFor(day)}
		{@const dayShifts = byDay.get(day) ?? []}
		<button
			type="button"
			onclick={() => onaddday?.(iso)}
			class="min-h-[4.5rem] rounded-lg border bg-white p-1.5 text-left {iso === today
				? 'border-blue-500 ring-1 ring-blue-500'
				: 'border-gray-200 hover:bg-gray-50'}"
		>
			<span class="block text-xs text-gray-500">{day}{iso === today ? ' • today' : ''}</span>
			{#each dayShifts as s (s.id)}
				<span
					role="button"
					tabindex="0"
					onclick={(e) => {
						e.stopPropagation();
						onopen?.(s);
					}}
					onkeydown={(e) => {
						if (e.key === 'Enter' || e.key === ' ') {
							e.stopPropagation();
							e.preventDefault();
							onopen?.(s);
						}
					}}
					class="mt-1 block truncate rounded px-1 py-0.5 text-[10.5px] font-medium {eventClass(
						s.status
					)}"
				>
					{chipLabel(s)}
				</span>
			{/each}
		</button>
	{/each}
</div>
