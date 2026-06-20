<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { shifts } from '$lib/stores/shifts.svelte';
	import { invoices } from '$lib/stores/invoices.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import * as shiftsApi from '$lib/api/shifts';
	import ShiftTable from '$lib/components/ShiftTable.svelte';
	import ShiftForm from '$lib/components/ShiftForm.svelte';
	import Calendar from '$lib/components/Calendar.svelte';
	import InvoiceSuggestions from '$lib/components/InvoiceSuggestions.svelte';
	import { todayISO } from '$lib/shifts/format';
	import type { Participant, ParticipantInput, Shift } from '$lib/api/types';

	const idParam = $derived(page.params.id === 'new' ? 'new' : Number(page.params.id));

	// ── Flat editable fields. `mgmtType` is an enum select; `planManagerId` is
	// relational (EntityEditor's select can't resolve names), so it's shown
	// read-only here and managed via the create flow / future relational picker. ──
	const flatCols: Column<Participant>[] = [
		{ key: 'name', label: 'Name', input: 'text' },
		{ key: 'ndisNumber', label: 'NDIS number', input: 'text' },
		{ key: 'planStart', label: 'Plan start', input: 'date' },
		{ key: 'planEnd', label: 'Plan end', input: 'date' },
		{ key: 'mgmtType', label: 'Management type', input: 'select', values: ['plan', 'self'] },
		{
			key: 'planManagerId',
			label: 'Plan manager',
			input: 'readonly',
			cell: (p) => p.planManagerName || '—'
		},
		{ key: 'email', label: 'Email', input: 'text' },
		{ key: 'phone', label: 'Phone', input: 'text' },
		{ key: 'address', label: 'Address', input: 'textarea' }
	];

	function toInput(p: Participant): ParticipantInput {
		return {
			name: p.name,
			ndisNumber: p.ndisNumber,
			planStart: p.planStart,
			planEnd: p.planEnd,
			mgmtType: p.mgmtType === 'self' ? 'self' : 'plan',
			planManagerId: p.mgmtType === 'self' ? null : p.planManagerId,
			email: p.email,
			phone: p.phone,
			address: p.address,
			metadata: p.metadata
		};
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'name' && String(value ?? '').trim() === '') return 'Name is required.';
		return null;
	}

	// ── Shifts / invoices section (extras) ───────────────────────────────────────
	onMount(() => {
		shifts.ensureSubscribed();
		void shifts.load();
		invoices.ensureSubscribed();
		void invoices.load();
		participants.ensureSubscribed();
		void participants.load();
	});

	function nameFor(id: number): string {
		const p = participants.items.find((x) => x.id === id);
		return p ? p.name : `#${id}`;
	}

	function money(n: number): string {
		return '$' + (Number.isFinite(n) ? n : 0).toFixed(2);
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

	const month = todayISO().slice(0, 7); // YYYY-MM
	let shiftView = $state<'table' | 'calendar'>('table');

	// ── Shift form ──
	let formOpen = $state(false);
	let formShift = $state<Shift | null>(null);
	let formRecording = $state(false);
	let formDate = $state('');
	let formParticipantId = $state(0);

	function openAdd(pid: number): void {
		formShift = null;
		formRecording = false;
		formDate = '';
		formParticipantId = pid;
		formOpen = true;
	}

	function addDay(pid: number, dateISO: string): void {
		formShift = null;
		formRecording = false;
		formDate = dateISO;
		formParticipantId = pid;
		formOpen = true;
	}

	function openShift(s: Shift): void {
		formShift = s;
		formRecording = s.status === 'scheduled';
		formParticipantId = s.participantId;
		formOpen = true;
	}

	function onSaved(): void {
		void shifts.load();
	}

	async function deleteShifts(ids: number[]): Promise<void> {
		for (const id of ids) {
			await shiftsApi.remove(id); // bounded by selection
		}
		await shifts.load();
	}
</script>

<EntityEditor
	title="Participant"
	columns={flatCols}
	crud={participants.crud}
	id={idParam}
	{toInput}
	{validate}
	backHref="/participants"
	{extras}
/>

{#snippet extras(row: Participant)}
	{#if row.id > 0}
		{@const myShifts = shifts.items.filter((s) => s.participantId === row.id)}
		{@const myInvoices = invoices.items.filter((i) => i.participantId === row.id)}
		<div class="space-y-6">
			<InvoiceSuggestions suggestions={shifts.suggestions} {nameFor} participantId={row.id} />

			<section class="space-y-3">
				<div class="flex flex-wrap items-center justify-between gap-3">
					<div class="flex items-center gap-2">
						<h2 class="text-xs font-semibold tracking-wide text-gray-500 uppercase">Shifts</h2>
						<div class="flex overflow-hidden rounded border border-gray-300 text-xs">
							<button
								type="button"
								onclick={() => (shiftView = 'table')}
								class="px-2.5 py-1 {shiftView === 'table'
									? 'bg-gray-900 text-white'
									: 'text-gray-600 hover:bg-gray-50'}">Table</button
							>
							<button
								type="button"
								onclick={() => (shiftView = 'calendar')}
								class="px-2.5 py-1 {shiftView === 'calendar'
									? 'bg-gray-900 text-white'
									: 'text-gray-600 hover:bg-gray-50'}">Calendar</button
							>
						</div>
					</div>
					<button
						type="button"
						onclick={() => openAdd(row.id)}
						class="shrink-0 rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
					>
						+ Ad-hoc shift
					</button>
				</div>

				{#if shiftView === 'table'}
					<ShiftTable
						shifts={myShifts}
						participantName={nameFor}
						onopen={openShift}
						ondelete={deleteShifts}
					/>
				{:else}
					<div class="rounded-lg border border-gray-200 bg-white p-3">
						<Calendar
							shifts={myShifts}
							{nameFor}
							{month}
							onaddday={(d) => addDay(row.id, d)}
							onopen={openShift}
						/>
					</div>
					<p class="text-xs text-gray-500">
						This participant's shifts this month. Click a day to add, a chip to edit or record.
					</p>
				{/if}
			</section>

			<section class="space-y-2">
				<h2 class="text-xs font-semibold tracking-wide text-gray-500 uppercase">Invoices</h2>
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
			</section>
		</div>
	{:else}
		<p class="text-sm text-gray-500">Save this participant to add shifts.</p>
	{/if}
{/snippet}

<ShiftForm
	bind:open={formOpen}
	shift={formShift}
	recording={formRecording}
	presetDate={formDate}
	presetParticipantId={formParticipantId}
	onsaved={onSaved}
/>
