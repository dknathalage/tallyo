<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { replaceState, goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import { createAutosave, type SaveState } from './autosave';
	import { sessions } from '$lib/stores/sessions.svelte';
	import Sparkle from '$lib/components/Sparkle.svelte';
	import { invoices } from '$lib/stores/invoices.svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { payers } from '$lib/stores/payers.svelte';
	import { features } from '$lib/stores/features.svelte';
	import * as sessionsApi from '$lib/api/sessions';
	import * as smarts from '$lib/api/smarts';
	import SessionTable from '$lib/components/SessionTable.svelte';
	import SessionForm from '$lib/components/SessionForm.svelte';
	import Calendar from '$lib/components/Calendar.svelte';
	import InvoiceSuggestions from '$lib/components/InvoiceSuggestions.svelte';
	import Button from '$lib/components/Button.svelte';
	import Badge from '$lib/components/Badge.svelte';
	import { todayISO } from '$lib/sessions/format';
	import type { Client, ClientInput, Session } from '$lib/api/types';

	type Props = {
		/** Existing client uuid, or 'new' to create. */
		idParam: string | 'new';
	};

	let { idParam }: Props = $props();

	// ── Editable fields (each its own $state, seeded from the client). ──
	// A client is generic: a name, contact details, an optional free-text
	// reference and an optional payer. (The former type, plan-window,
	// management type — were removed.)
	let name = $state('');
	let reference = $state('');
	// payerId is held as the <select> string ('' = none) and coerced to a
	// payer uuid in buildInput. It is relational (id → name), which is why this
	// page manages it directly rather than via the generic flat editor.
	let payer = $state('');
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
	// or set in onCreated) — gates the sessions/invoices section, mirroring the old
	// `row.id > 0` guard.
	// svelte-ignore state_referenced_locally -- intentional one-time seed; this component is remounted by a {#key idParam} on the route, so an id change is handled by remount, not reactivity. onCreated owns currentId thereafter.
	let currentId = $state<string>(idParam === 'new' ? '' : idParam);

	function seedFrom(p: Client): void {
		name = p.name;
		reference = p.reference;
		payer = p.payerId === null ? '' : String(p.payerId);
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
	// payerId can be reverted by the other's autosave.
	function buildInput(): ClientInput {
		return {
			name,
			reference,
			payerId: payer === '' ? null : payer,
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

	// Manual save — forces the current (valid) fields to persist immediately.
	function saveNow(): void {
		nameError = name.trim() === '' ? 'Name is required.' : null;
		if (nameError) return;
		autosave.schedule(buildInput());
		autosave.flush();
	}

	// ── Sessions / invoices section ──
	onMount(() => {
		sessions.ensureSubscribed();
		void sessions.load();
		invoices.ensureSubscribed();
		void invoices.load();
		clients.ensureSubscribed();
		void clients.load();
		payers.ensureSubscribed();
		void payers.query({ page: 1, limit: 500 });
	});

	function nameFor(id: string): string {
		const p = clients.items.find((x) => x.id === id);
		return p ? p.name : `#${id}`;
	}

	function money(n: number): string {
		return '$' + (Number.isFinite(n) ? n : 0).toFixed(2);
	}

	function invStatusTone(s: string): 'green' | 'brand' | 'red' | 'gray' {
		switch (s) {
			case 'paid':
				return 'green';
			case 'sent':
				return 'brand';
			case 'overdue':
				return 'red';
			default:
				return 'gray';
		}
	}

	const month = todayISO().slice(0, 7); // YYYY-MM
	let sessionView = $state<'table' | 'calendar'>('table');

	// ── Session form ──
	let formOpen = $state(false);
	let formSession = $state<Session | null>(null);
	let formRecording = $state(false);
	let formDate = $state('');
	let formClientId = $state('');

	function openAdd(pid: string): void {
		formSession = null;
		formRecording = false;
		formDate = '';
		formClientId = pid;
		formOpen = true;
	}

	function addDay(pid: string, dateISO: string): void {
		formSession = null;
		formRecording = false;
		formDate = dateISO;
		formClientId = pid;
		formOpen = true;
	}

	function openSession(s: Session): void {
		formSession = s;
		formRecording = s.status === 'scheduled';
		formClientId = s.clientId;
		formOpen = true;
	}

	function onSaved(): void {
		void sessions.load();
	}

	async function deleteSessions(ids: string[]): Promise<void> {
		for (const id of ids) {
			await sessionsApi.remove(id); // bounded by selection
		}
		await sessions.load();
	}

	// ── AI: draft a blank invoice for this client ──
	let aiDrafting = $state(false);
	let aiDraftError = $state<string | null>(null);

	async function draftInvoiceWithAI(): Promise<void> {
		if (currentId === '') return;
		aiDraftError = null;
		aiDrafting = true;
		try {
			const newId = await smarts.draftInvoice(currentId);
			await goto(t(`/invoices/${newId}`));
		} catch (err) {
			aiDraftError = err instanceof Error ? err.message : 'Failed to draft the invoice.';
		} finally {
			aiDrafting = false;
		}
	}

	const mySessions = $derived(sessions.items.filter((s) => s.clientId === currentId));
	const myInvoices = $derived(invoices.items.filter((i) => i.clientId === currentId));
	const payerOptions = $derived(payers.items);
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
			<Button
				size="sm"
				onclick={saveNow}
				loading={status === 'saving'}
				disabled={!loaded || status === 'saving'}
			>
				Save
			</Button>
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
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				/>
				{#if nameError}<span class="mt-1 block text-xs text-red-600">{nameError}</span>{/if}
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Reference</span>
				<input
					type="text"
					bind:value={reference}
					oninput={changed}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Payer</span>
				<select
					value={payer}
					onchange={(e) => {
						payer = e.currentTarget.value;
						changed();
					}}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="">— none —</option>
					{#each payerOptions as pm (pm.id)}
						<option value={String(pm.id)}>{pm.name}</option>
					{/each}
				</select>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Email</span>
				<input
					type="text"
					bind:value={email}
					oninput={changed}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Phone</span>
				<input
					type="text"
					bind:value={phone}
					oninput={changed}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			<label class="block">
				<span class="mb-1 block text-sm font-medium">Address</span>
				<textarea
					bind:value={address}
					oninput={changed}
					rows="3"
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
				></textarea>
			</label>
		</div>

		{#if currentId !== ''}
			<div class="space-y-6">
				<InvoiceSuggestions suggestions={sessions.suggestions} {nameFor} clientId={currentId} />

				<section class="space-y-3">
					<div class="flex flex-wrap items-center justify-between gap-3">
						<div class="flex items-center gap-2">
							<h2 class="text-xs font-semibold tracking-wide text-gray-500 uppercase">Sessions</h2>
							<div class="flex overflow-hidden rounded-lg border border-gray-300 text-xs">
								<button
									type="button"
									onclick={() => (sessionView = 'table')}
									class="px-2.5 py-1 {sessionView === 'table'
										? 'bg-brand-700 text-onbrand'
										: 'text-gray-600 hover:bg-gray-50'}">Table</button
								>
								<button
									type="button"
									onclick={() => (sessionView = 'calendar')}
									class="px-2.5 py-1 {sessionView === 'calendar'
										? 'bg-brand-700 text-onbrand'
										: 'text-gray-600 hover:bg-gray-50'}">Calendar</button
								>
							</div>
						</div>
						<Button onclick={() => openAdd(currentId)}>
							+ Ad-hoc session
						</Button>
					</div>

					{#if sessionView === 'table'}
						<SessionTable
							sessions={mySessions}
							clientName={nameFor}
							onopen={openSession}
							ondelete={deleteSessions}
						/>
					{:else}
						<div class="rounded-lg border border-gray-200 bg-white p-3">
							<Calendar
								sessions={mySessions}
								{nameFor}
								{month}
								onaddday={(d) => addDay(currentId, d)}
								onopen={openSession}
							/>
						</div>
						<p class="text-xs text-gray-500">
							This client's sessions this month. Click a day to add, a chip to edit or record.
						</p>
					{/if}
				</section>

				<section class="space-y-2">
					<div class="flex items-center justify-between gap-3">
						<h2 class="text-xs font-semibold tracking-wide text-gray-500 uppercase">Invoices</h2>
						{#if features.smarts}
							<Button
								variant="secondary"
								size="sm"
								loading={aiDrafting}
								disabled={aiDrafting}
								onclick={draftInvoiceWithAI}
							>
								<Sparkle /> Draft invoice with AI
							</Button>
						{/if}
					</div>
					{#if aiDraftError}<p class="text-sm text-red-600">{aiDraftError}</p>{/if}
					{#each myInvoices as inv (inv.id)}
						<a
							href={t(`/invoices/${inv.id}`)}
							class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 hover:border-gray-400"
						>
							<span>
								<b class="font-mono tabular-nums">{inv.number}</b>
								<span class="ml-2 inline-block capitalize">
									<Badge tone={invStatusTone(inv.status)}>{inv.status}</Badge>
								</span>
								<span class="block text-sm text-gray-500 font-mono tabular-nums">
									{inv.issueDate ? inv.issueDate.slice(0, 10) : '—'}
								</span>
							</span>
							<span class="font-semibold font-mono tabular-nums">{money(inv.total)} ›</span>
						</a>
					{:else}
						<p class="text-sm text-gray-500">No invoices yet.</p>
					{/each}
				</section>
			</div>
		{:else}
			<p class="text-sm text-gray-500">Save this client to add sessions.</p>
		{/if}
	{/if}
</div>

<SessionForm
	bind:open={formOpen}
	session={formSession}
	recording={formRecording}
	presetDate={formDate}
	presetClientId={formClientId}
	onsaved={onSaved}
/>
