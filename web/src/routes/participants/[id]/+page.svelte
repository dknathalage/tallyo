<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { shifts } from '$lib/stores/shifts.svelte';
	import { invoices } from '$lib/stores/invoices.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import ShiftTable from '$lib/components/ShiftTable.svelte';
	import ShiftForm from '$lib/components/ShiftForm.svelte';
	import InvoiceSuggestions from '$lib/components/InvoiceSuggestions.svelte';
	import Calendar from '$lib/components/Calendar.svelte';
	import { todayISO } from '$lib/shifts/format';
	import type { Shift } from '$lib/api/types';

	const participantId = $derived(Number(page.params.id));

	type Tab = 'shifts' | 'calendar' | 'invoices' | 'details';
	let tab = $state<Tab>('shifts');

	onMount(() => {
		shifts.ensureSubscribed();
		void shifts.load();
		invoices.ensureSubscribed();
		void invoices.load();
		participants.ensureSubscribed();
		void participants.load();
	});

	const participant = $derived(participants.items.find((p) => p.id === participantId) ?? null);

	function nameFor(id: number): string {
		const p = participants.items.find((x) => x.id === id);
		return p ? p.name : `#${id}`;
	}

	const myShifts = $derived(shifts.items.filter((s) => s.participantId === participantId));
	const myInvoices = $derived(invoices.items.filter((i) => i.participantId === participantId));

	function money(n: number): string {
		return '$' + (Number.isFinite(n) ? n : 0).toFixed(2);
	}

	// ---- Calendar (current month) ----
	const month = todayISO().slice(0, 7); // YYYY-MM

	// ---- Shift form ----
	let formOpen = $state(false);
	let formShift = $state<Shift | null>(null);
	let formRecording = $state(false);
	let formDate = $state('');

	function openAdd(): void {
		formShift = null;
		formRecording = false;
		formDate = '';
		formOpen = true;
	}

	function addDay(dateISO: string): void {
		formShift = null;
		formRecording = false;
		formDate = dateISO;
		formOpen = true;
	}

	function openShift(s: Shift): void {
		formShift = s;
		formRecording = s.status === 'scheduled';
		formOpen = true;
	}

	function onSaved(): void {
		void shifts.load();
	}

	function invStatusClass(status: string): string {
		switch (status) {
			case 'paid':
				return 'bg-green-100 text-green-800';
			case 'sent':
				return 'bg-blue-100 text-blue-800';
			case 'overdue':
				return 'bg-red-100 text-red-800';
			default:
				return 'bg-gray-100 text-gray-700';
		}
	}

	const TABS: { id: Tab; label: string }[] = [
		{ id: 'shifts', label: 'Shifts' },
		{ id: 'calendar', label: 'Calendar' },
		{ id: 'invoices', label: 'Invoices' },
		{ id: 'details', label: 'Details' }
	];
</script>

<div class="space-y-6">
	<p class="text-sm text-gray-500">
		<a href="/participants" class="text-blue-600 hover:underline">Participants</a>
		<span> › {participant?.name ?? 'Participant'}</span>
	</p>

	<div class="flex flex-wrap items-start justify-between gap-3">
		<div>
			<h1 class="text-xl font-semibold">{participant?.name ?? `Participant #${participantId}`}</h1>
			{#if participant}
				<p class="text-sm text-gray-500">
					NDIS {participant.ndisNumber || '—'} ·
					{participant.mgmtType === 'self' ? 'Self-managed' : 'Plan-managed'}
				</p>
			{/if}
		</div>
		<button
			type="button"
			onclick={openAdd}
			class="shrink-0 rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
		>
			+ Ad-hoc shift
		</button>
	</div>

	<InvoiceSuggestions suggestions={shifts.suggestions} {nameFor} {participantId} />

	<nav class="flex flex-wrap gap-x-1 border-b border-gray-200">
		{#each TABS as t (t.id)}
			<button
				type="button"
				onclick={() => (tab = t.id)}
				aria-current={tab === t.id ? 'page' : undefined}
				class="-mb-px border-b-2 px-3 py-2 text-sm {tab === t.id
					? 'border-gray-900 font-medium text-gray-900'
					: 'border-transparent text-gray-600 hover:text-gray-900'}"
			>
				{t.label}
			</button>
		{/each}
	</nav>

	{#if tab === 'shifts'}
		<ShiftTable shifts={myShifts} participantName={nameFor} onopen={openShift} />
	{:else if tab === 'calendar'}
		<div class="rounded-lg border border-gray-200 bg-white p-3">
			<Calendar shifts={myShifts} {nameFor} {month} onaddday={addDay} onopen={openShift} />
		</div>
		<p class="text-xs text-gray-500">
			This participant's shifts this month. Click a day to add, a chip to edit or record.
		</p>
	{:else if tab === 'invoices'}
		<div class="space-y-2">
			{#each myInvoices as inv (inv.id)}
				<a
					href={`/invoices/${inv.id}`}
					class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 hover:border-gray-400"
				>
					<span>
						<b>{inv.number}</b>
						<span
							class="ml-2 inline-block rounded px-2 py-0.5 text-xs font-medium capitalize {invStatusClass(
								inv.status
							)}">{inv.status}</span
						>
						<span class="block text-sm text-gray-500">
							{inv.issueDate ? inv.issueDate.slice(0, 10) : '—'}
						</span>
					</span>
					<span class="font-semibold">{money(inv.total)} ›</span>
				</a>
			{:else}
				<p class="text-sm text-gray-500">No invoices yet.</p>
			{/each}
		</div>
	{:else if tab === 'details'}
		{#if participant}
			<dl class="grid max-w-lg grid-cols-[10rem_1fr] gap-y-2 rounded-lg border border-gray-200 bg-white p-4 text-sm">
				<dt class="text-gray-500">NDIS number</dt>
				<dd>{participant.ndisNumber || '—'}</dd>
				<dt class="text-gray-500">Management</dt>
				<dd>{participant.mgmtType === 'self' ? 'Self-managed' : 'Plan-managed'}</dd>
				<dt class="text-gray-500">Plan manager</dt>
				<dd>{participant.planManagerName || '—'}</dd>
				<dt class="text-gray-500">Plan window</dt>
				<dd>
					{participant.planStart ? participant.planStart.slice(0, 10) : '—'} – {participant.planEnd
						? participant.planEnd.slice(0, 10)
						: '—'}
				</dd>
				<dt class="text-gray-500">Email</dt>
				<dd>{participant.email || '—'}</dd>
				<dt class="text-gray-500">Phone</dt>
				<dd>{participant.phone || '—'}</dd>
				<dt class="text-gray-500">Address</dt>
				<dd>{participant.address || '—'}</dd>
			</dl>
		{:else}
			<p class="text-sm text-gray-500">Participant not found.</p>
		{/if}
	{/if}
</div>

<ShiftForm
	bind:open={formOpen}
	shift={formShift}
	recording={formRecording}
	presetDate={formDate}
	presetParticipantId={participantId}
	onsaved={onSaved}
/>
