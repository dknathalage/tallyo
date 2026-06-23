<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { t } from '$lib/nav';
	import { createAutosave, type SaveState } from './autosave';
	import { shifts } from '$lib/stores/shifts.svelte';
	import { invoices } from '$lib/stores/invoices.svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { planManagers } from '$lib/stores/planManagers.svelte';
	import * as shiftsApi from '$lib/api/shifts';
	import ShiftTable from '$lib/components/ShiftTable.svelte';
	import ShiftForm from '$lib/components/ShiftForm.svelte';
	import Calendar from '$lib/components/Calendar.svelte';
	import InvoiceSuggestions from '$lib/components/InvoiceSuggestions.svelte';
	import { todayISO } from '$lib/shifts/format';
	import type { Client, ClientInput, Shift } from '$lib/api/types';

	type Props = {
		/** Existing client uuid, or 'new' to create. */
		idParam: string | 'new';
	};

	let { idParam }: Props = $props();

	// ── Editable fields (each its own $state, seeded from the client). ──
	let name = $state('');
	// type defaults to 'standard'; conditional show/hide of NDIS-only fields by
	// type is deferred to a later phase — this is just a plain select for now.
	let clientType = $state<'ndis' | 'standard'>('standard');
	let reference = $state('');
	let planStart = $state('');
	let planEnd = $state('');
	let mgmtType = $state<'plan' | 'self'>('plan');
	// planManagerId is held as the <select> string ('' = none) and coerced to a
	// number id in buildInput. It is relational (id → name), which is why this
	// page manages it directly rather than via the generic flat editor.
	let planManager = $state('');
	let email = $state('');
	let phone = $state('');
	let address = $state('');
	let metadata = $state(''); // writable but not surfaced — preserved verbatim.

	// ── Load / status state ──
	let loadError = $state<string | null>(null);
	let loaded = $state(false);
	let nameError = $state<string | null>(null);
	let status = $state<SaveState>('idle');
	// currentId is the client uuid once the record exists ('' until created,
	// or set in onCreated) — gates the shifts/invoices section, mirroring the old
	// `row.id > 0` guard.
	// svelte-ignore state_referenced_locally -- intentional one-time seed; this component is remounted by a {#key idParam} on the route, so an id change is handled by remount, not reactivity. onCreated owns currentId thereafter.
	let currentId = $state<string>(idParam === 'new' ? '' : idParam);

	function seedFrom(p: Client): void {
		name = p.name;
		clientType = p.type === 'ndis' ? 'ndis' : 'standard';
		reference = p.reference;
		planStart = p.planStart;
		planEnd = p.planEnd;
		mgmtType = p.mgmtType === 'self' ? 'self' : 'plan';
		planManager = p.planManagerId === null ? '' : String(p.planManagerId);
		email = p.email;
		phone = p.phone;
		address = p.address;
		metadata = p.metadata;
	}

	async function init(): Promise<void> {
		if (idParam === 'new') {
			loaded = true;
			return;
		}
		try {
			const p = await clients.crud.get(idParam);
			seedFrom(p);
			loaded = true;
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to load client.';
		}
	}
	void init();

	// ── Autosave wiring ──
	const autosave = createAutosave<ClientInput, Client>({
		// svelte-ignore state_referenced_locally -- intentional one-time seed (remounted by {#key idParam}).
		initialId: idParam === 'new' ? null : idParam,
		create: (input) => clients.crud.create(input),
		update: (id, input) => clients.crud.update(id, input),
		onState: (s) => (status = s),
		onCreated: (newId) => {
			currentId = newId;
			replaceState(t(`/clients/${newId}`), {});
		}
	});
	onDestroy(() => autosave.dispose());

	// buildInput assembles the FULL writable payload from current field state, so a
	// save of ANY field carries the latest value of EVERY field — neither name nor
	// planManagerId can be reverted by the other's autosave.
	function buildInput(): ClientInput {
		const pmId = planManager === '' ? null : planManager;
		return {
			name,
			type: clientType,
			reference,
			planStart,
			planEnd,
			mgmtType,
			planManagerId: mgmtType === 'self' ? null : pmId,
			email,
			phone,
			address,
			metadata
		};
	}

	// Called on every field change: validate name, then schedule a debounced save.
	function changed(): void {
		nameError = name.trim() === '' ? 'Name is required.' : null;
		if (nameError) return;
		autosave.schedule(buildInput());
	}

	function onMgmtTypeChange(v: string): void {
		mgmtType = v === 'self' ? 'self' : 'plan';
		if (mgmtType === 'self') planManager = ''; // force none when self-managed
		changed();
	}

	// Manual save — forces the current (valid) fields to persist immediately.
	function saveNow(): void {
		nameError = name.trim() === '' ? 'Name is required.' : null;
		if (nameError) return;
		autosave.schedule(buildInput());
		autosave.flush();
	}

	// ── Shifts / invoices section ──
	onMount(() => {
		shifts.ensureSubscribed();
		void shifts.load();
		invoices.ensureSubscribed();
		void invoices.load();
		clients.ensureSubscribed();
		void clients.load();
		planManagers.ensureSubscribed();
		void planManagers.query({ page: 1, limit: 500 });
	});

	function nameFor(id: string): string {
		const p = clients.items.find((x) => x.id === id);
		return p ? p.name : `#${id}`;
	}

	function money(n: number): string {
		return '$' + (Number.isFinite(n) ? n : 0).toFixed(2);
	}

	function invStatusClass(s: string): string {
		switch (s) {
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
	let formClientId = $state('');

	function openAdd(pid: string): void {
		formShift = null;
		formRecording = false;
		formDate = '';
		formClientId = pid;
		formOpen = true;
	}

	function addDay(pid: string, dateISO: string): void {
		formShift = null;
		formRecording = false;
		formDate = dateISO;
		formClientId = pid;
		formOpen = true;
	}

	function openShift(s: Shift): void {
		formShift = s;
		formRecording = s.status === 'scheduled';
		formClientId = s.clientId;
		formOpen = true;
	}

	function onSaved(): void {
		void shifts.load();
	}

	async function deleteShifts(ids: string[]): Promise<void> {
		for (const id of ids) {
			await shiftsApi.remove(id); // bounded by selection
		}
		await shifts.load();
	}

	const myShifts = $derived(shifts.items.filter((s) => s.clientId === currentId));
	const myInvoices = $derived(invoices.items.filter((i) => i.clientId === currentId));
	const planManagerOptions = $derived(planManagers.items);
</script>

<div class="space-y-5">
	<div class="flex items-center justify-between">
		<a href={t('/clients')} class="text-sm text-gray-500 hover:text-gray-900">← Back</a>
		<div class="flex items-center gap-3">
			<span class="h-4 text-xs">
				{#if status === 'saving'}<span class="text-gray-400">saving…</span>
				{:else if status === 'saved'}<span class="text-green-600">✓ saved</span>
				{:else if status === 'error'}
					<span class="text-red-600"
						>⚠ error ·
						<button type="button" class="underline" onclick={() => autosave.retry()}>retry</button>
					</span>
				{/if}
			</span>
			<button
				type="button"
				onclick={saveNow}
				disabled={!loaded || status === 'saving'}
				class="rounded bg-gray-900 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
			>
				Save
			</button>
		</div>
	</div>

	<h1 class="text-xl font-semibold">{idParam === 'new' ? 'New Client' : 'Client'}</h1>

	{#if loadError}
		<p class="text-sm text-red-600">{loadError}</p>
	{:else if !loaded}
		<p class="text-sm text-gray-500">Loading…</p>
	{:else}
		<div class="max-w-xl space-y-4">
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Name</span>
				<input
					type="text"
					bind:value={name}
					oninput={changed}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
				{#if nameError}<span class="mt-1 block text-xs text-red-600">{nameError}</span>{/if}
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Type</span>
				<select
					value={clientType}
					onchange={(e) => {
						clientType = e.currentTarget.value === 'ndis' ? 'ndis' : 'standard';
						changed();
					}}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="standard">Standard</option>
					<option value="ndis">NDIS</option>
				</select>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Reference</span>
				<input
					type="text"
					bind:value={reference}
					oninput={changed}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Plan start</span>
				<input
					type="date"
					bind:value={planStart}
					oninput={changed}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Plan end</span>
				<input
					type="date"
					bind:value={planEnd}
					oninput={changed}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Management type</span>
				<select
					value={mgmtType}
					onchange={(e) => onMgmtTypeChange(e.currentTarget.value)}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="plan">Plan-managed</option>
					<option value="self">Self-managed</option>
				</select>
			</label>

			{#if mgmtType !== 'self'}
				<label class="block">
					<span class="mb-1 block text-sm font-medium">Plan manager</span>
					<select
						value={planManager}
						onchange={(e) => {
							planManager = e.currentTarget.value;
							changed();
						}}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					>
						<option value="">— none —</option>
						{#each planManagerOptions as pm (pm.id)}
							<option value={String(pm.id)}>{pm.name}</option>
						{/each}
					</select>
				</label>
			{/if}

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Email</span>
				<input
					type="text"
					bind:value={email}
					oninput={changed}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Phone</span>
				<input
					type="text"
					bind:value={phone}
					oninput={changed}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Address</span>
				<textarea
					bind:value={address}
					oninput={changed}
					rows="3"
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				></textarea>
			</label>
		</div>

		{#if currentId !== ''}
			<div class="space-y-6">
				<InvoiceSuggestions suggestions={shifts.suggestions} {nameFor} clientId={currentId} />

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
							onclick={() => openAdd(currentId)}
							class="shrink-0 rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
						>
							+ Ad-hoc shift
						</button>
					</div>

					{#if shiftView === 'table'}
						<ShiftTable
							shifts={myShifts}
							clientName={nameFor}
							onopen={openShift}
							ondelete={deleteShifts}
						/>
					{:else}
						<div class="rounded-lg border border-gray-200 bg-white p-3">
							<Calendar
								shifts={myShifts}
								{nameFor}
								{month}
								onaddday={(d) => addDay(currentId, d)}
								onopen={openShift}
							/>
						</div>
						<p class="text-xs text-gray-500">
							This client's shifts this month. Click a day to add, a chip to edit or record.
						</p>
					{/if}
				</section>

				<section class="space-y-2">
					<h2 class="text-xs font-semibold tracking-wide text-gray-500 uppercase">Invoices</h2>
					{#each myInvoices as inv (inv.id)}
						<a
							href={t(`/invoices/${inv.id}`)}
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
			<p class="text-sm text-gray-500">Save this client to add shifts.</p>
		{/if}
	{/if}
</div>

<ShiftForm
	bind:open={formOpen}
	shift={formShift}
	recording={formRecording}
	presetDate={formDate}
	presetClientId={formClientId}
	onsaved={onSaved}
/>
