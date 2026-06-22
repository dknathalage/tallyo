<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { supportCatalog } from '$lib/stores/supportCatalog.svelte';
	import { features } from '$lib/stores/features.svelte';
	import * as shiftsApi from '$lib/api/shifts';
	import { hoursBetween, todayISO } from '$lib/shifts/format';
	import type {
		Shift,
		ShiftInput,
		ShiftStatus,
		LineItem,
		LineItemInput,
		SupportItem
	} from '$lib/api/types';

	type Props = {
		open?: boolean;
		/** Render the form body without the Modal wrapper (full-page route host). */
		inline?: boolean;
		/** Editing target — null for a fresh shift. */
		shift?: Shift | null;
		/** Pre-filled date for a fresh shift (e.g. clicked calendar day). */
		presetDate?: string;
		/** Pre-selected participant (uuid) for a fresh shift. */
		presetParticipantId?: string | null;
		/** Recording a scheduled shift — adjusts the title + advances status. */
		recording?: boolean;
		/** Called after a successful save (create/update). */
		onsaved?: () => void;
		/** Inline Cancel/Back action (modal mode just closes via open). */
		oncancel?: () => void;
	};

	let {
		open = $bindable(false),
		inline = false,
		shift = null,
		presetDate = '',
		presetParticipantId = null,
		recording = false,
		onsaved,
		oncancel
	}: Props = $props();

	const editing = $derived(shift !== null);
	const title = $derived(recording ? 'Record shift' : editing ? 'Edit shift' : 'Add shift');
	const saveLabel = $derived(recording ? 'Save recording' : editing ? 'Save' : 'Add');

	// Form fields. Re-seeded whenever the modal opens (the form is reused).
	let fParticipantId = $state('');
	let fDate = $state('');
	let fNote = $state('');
	let fStatus = $state<ShiftStatus>('recorded');
	let saving = $state(false);
	let error = $state<string | null>(null);

	// Shift line items (loaded for an existing shift only — a new shift is saved
	// note-only, then re-opened to add items). Each add/remove hits the API and
	// refetches; the server prices coded lines authoritatively.
	let items = $state<LineItem[]>([]);
	let itemsBusy = $state(false);
	let itemError = $state<string | null>(null);

	// Seed the form from props. In modal mode, re-seed each time the modal
	// transitions to open (the form is reused). In inline mode the host route
	// remounts the component (via {#key idParam}), so seed once on mount.
	let lastOpen = false;
	let seeded = false;
	$effect(() => {
		if (inline) {
			if (!seeded) {
				seed();
				seeded = true;
			}
			return;
		}
		if (open && !lastOpen) {
			seed();
		}
		lastOpen = open;
	});

	function seed(): void {
		error = null;
		itemError = null;
		items = [];
		resetDraft();
		if (shift) {
			fParticipantId = String(shift.participantId);
			fDate = shift.serviceDate ? shift.serviceDate.slice(0, 10) : todayISO();
			fNote = shift.note;
			fStatus = shift.status;
			void loadItems(shift.id);
		} else {
			const first = presetParticipantId ?? participants.items[0]?.id ?? null;
			fParticipantId = first === null ? '' : String(first);
			fDate = presetDate || todayISO();
			fNote = '';
			fStatus = 'recorded';
		}
	}

	async function loadItems(shiftId: string): Promise<void> {
		try {
			items = await shiftsApi.listItems(shiftId);
		} catch (err) {
			itemError = err instanceof Error ? err.message : 'Failed to load items.';
		}
	}

	async function divideAI(): Promise<void> {
		if (!shift) return;
		itemError = null;
		itemsBusy = true;
		try {
			items = await shiftsApi.divideShift(shift.id);
		} catch (err) {
			itemError = err instanceof Error ? err.message : 'AI could not divide this shift.';
		} finally {
			itemsBusy = false;
		}
	}

	async function removeItem(id: string): Promise<void> {
		if (!shift) return;
		itemError = null;
		itemsBusy = true;
		try {
			await shiftsApi.deleteItem(shift.id, id);
			await loadItems(shift.id);
		} catch (err) {
			itemError = err instanceof Error ? err.message : 'Failed to remove item.';
		} finally {
			itemsBusy = false;
		}
	}

	// ---- New-item draft + catalogue picker ----
	// Mirrors billing.Classify: how a unit's quantity is captured.
	function unitClass(unit: string): 'time' | 'distance' | 'count' {
		const u = unit.trim().toUpperCase();
		if (u === 'H' || u === 'HOUR' || u === 'HR') return 'time';
		if (u === 'KM') return 'distance';
		return 'count';
	}

	let niCode = $state('');
	let niCustomItemId = $state<string | null>(null);
	let niDescription = $state('');
	let niUnit = $state('');
	let niQuantity = $state('1');
	let niUnitPrice = $state(''); // custom lines only; coded lines priced by server
	let niStart = $state('');
	let niEnd = $state('');

	function resetDraft(): void {
		niCode = '';
		niCustomItemId = null;
		niDescription = '';
		niUnit = '';
		niQuantity = '1';
		niUnitPrice = '';
		niStart = '';
		niEnd = '';
		pickerOpen = false;
	}

	function onDraftTime(): void {
		if (niStart && niEnd) niQuantity = String(hoursBetween(niStart, niEnd));
	}

	let pickerOpen = $state(false);
	let pickerSearch = $state('');
	let catalogItems = $state<SupportItem[]>([]);
	let catalogLoaded = $state(false);

	async function ensureCatalog(): Promise<void> {
		if (catalogLoaded) return;
		catalogLoaded = true;
		await supportCatalog.loadVersions();
		if (supportCatalog.versions.length > 0) {
			try {
				catalogItems = await supportCatalog.loadItems(supportCatalog.versions[0].id);
			} catch {
				catalogItems = [];
			}
		}
	}

	const pickerResults = $derived.by<SupportItem[]>(() => {
		const q = pickerSearch.trim().toLowerCase();
		if (q === '') return catalogItems.slice(0, 20);
		return catalogItems
			.filter((it) => it.code.toLowerCase().includes(q) || it.name.toLowerCase().includes(q))
			.slice(0, 20);
	});

	async function openPicker(): Promise<void> {
		pickerOpen = !pickerOpen;
		pickerSearch = '';
		if (pickerOpen) await ensureCatalog();
	}

	function pickItem(it: SupportItem): void {
		niCode = it.code;
		niCustomItemId = null;
		niDescription = it.name;
		niUnit = it.unit;
		niUnitPrice = '';
		pickerOpen = false;
	}

	async function addItem(): Promise<void> {
		if (!shift) return;
		itemError = null;
		const qty = Number(niQuantity) || 0;
		if (qty <= 0) {
			itemError = 'Quantity must be greater than zero.';
			return;
		}
		if (niDescription.trim() === '') {
			itemError = 'A description is required.';
			return;
		}
		const coded = niCode.trim() !== '';
		const input: LineItemInput = {
			supportItemId: null,
			customItemId: niCustomItemId,
			catalogVersionId: null,
			code: coded ? niCode.trim() : '',
			description: niDescription.trim(),
			serviceDate: fDate,
			unit: niUnit,
			startTime: unitClass(niUnit) === 'time' ? niStart : '',
			endTime: unitClass(niUnit) === 'time' ? niEnd : '',
			quantity: qty,
			unitPrice: coded ? 0 : Number(niUnitPrice) || 0,
			taxable: false,
			sortOrder: items.length
		};
		itemsBusy = true;
		try {
			await shiftsApi.addItem(shift.id, input);
			await loadItems(shift.id);
			resetDraft();
		} catch (err) {
			itemError = err instanceof Error ? err.message : 'Failed to add item.';
		} finally {
			itemsBusy = false;
		}
	}

	function money(n: number): string {
		return (Number.isFinite(n) ? n : 0).toFixed(2);
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
		const nextStatus: ShiftStatus = recording && fStatus === 'scheduled' ? 'recorded' : fStatus;
		const input: ShiftInput = {
			participantId: fParticipantId,
			serviceDate: fDate,
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
			if (!inline) open = false;
			onsaved?.();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save shift.';
		} finally {
			saving = false;
		}
	}
</script>

{#snippet body()}
	<form class="space-y-3" onsubmit={submit}>
		<p class="text-xs text-gray-500">
			Date &amp; participant are structured; the note is free text. Add billable line items below
			(or let AI divide the note into them).
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

		<label class="block">
			<span class="mb-1 block text-sm font-medium">Note</span>
			<textarea
				bind:value={fNote}
				rows="3"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			></textarea>
		</label>

		{#if editing && shift}
			<div class="rounded border border-gray-200 p-3">
				<div class="mb-2 flex items-center justify-between">
					<span class="text-sm font-medium">Line items</span>
					{#if features.agent}
						<button
							type="button"
							onclick={divideAI}
							disabled={itemsBusy}
							class="rounded border border-indigo-300 px-3 py-1 text-sm text-indigo-700 hover:bg-indigo-50 disabled:opacity-50"
						>
							{itemsBusy ? 'Working…' : 'Divide with AI'}
						</button>
					{/if}
				</div>

				{#if itemError}
					<p class="mb-2 text-sm text-red-600">{itemError}</p>
				{/if}

				{#each items as it (it.id)}
					<div class="flex items-center gap-2 border-b border-gray-100 py-1.5 text-sm last:border-0">
						<span class="flex-1">
							{#if it.code}<span class="font-mono text-xs text-gray-500">{it.code}</span> {/if}
							{it.description}
						</span>
						<span class="text-gray-500">{it.quantity} {it.unit}</span>
						<span class="w-20 text-right">${money(it.lineTotal)}</span>
						{#if it.invoiceId === null}
							<button
								type="button"
								onclick={() => removeItem(it.id)}
								disabled={itemsBusy}
								class="text-red-600 hover:underline disabled:opacity-50"
								aria-label="Remove item"
							>
								✕
							</button>
						{:else}
							<span class="text-xs text-gray-400" title="Billed — linked to an invoice">billed</span>
						{/if}
					</div>
				{:else}
					<p class="text-sm text-gray-500">No items yet.</p>
				{/each}

				<!-- Add-item draft row -->
				<div class="mt-3 space-y-2 rounded bg-gray-50 p-2">
					<div class="flex gap-1">
						<input
							type="text"
							bind:value={niCode}
							placeholder="NDIS code (optional)"
							class="w-44 rounded border border-gray-300 px-2 py-1 font-mono text-xs"
						/>
						<button
							type="button"
							onclick={openPicker}
							class="shrink-0 rounded border border-gray-300 px-2 text-xs hover:bg-white"
						>
							Find
						</button>
						<input
							type="text"
							bind:value={niDescription}
							placeholder="Description"
							class="flex-1 rounded border border-gray-300 px-2 py-1 text-sm"
						/>
					</div>

					{#if pickerOpen}
						<div class="rounded border border-gray-200 bg-white p-2">
							<input
								type="text"
								bind:value={pickerSearch}
								placeholder="Search by code or name"
								class="mb-2 w-full rounded border border-gray-300 px-2 py-1 text-sm"
							/>
							{#if !catalogLoaded}
								<p class="text-xs text-gray-500">Loading catalogue…</p>
							{:else if catalogItems.length === 0}
								<p class="text-xs text-gray-500">No catalogue loaded.</p>
							{:else}
								<ul class="max-h-40 overflow-auto text-sm">
									{#each pickerResults as it (it.id)}
										<li>
											<button
												type="button"
												onclick={() => pickItem(it)}
												class="w-full rounded px-2 py-1 text-left hover:bg-gray-50"
											>
												<span class="font-mono text-xs">{it.code}</span> — {it.name}
											</button>
										</li>
									{:else}
										<li class="px-2 py-1 text-xs text-gray-500">No matches.</li>
									{/each}
								</ul>
							{/if}
						</div>
					{/if}

					<div class="flex flex-wrap items-end gap-2">
						<label class="block">
							<span class="mb-0.5 block text-xs text-gray-500">Unit</span>
							<input
								type="text"
								bind:value={niUnit}
								placeholder="H, KM, EA…"
								class="w-20 rounded border border-gray-300 px-2 py-1 text-sm"
							/>
						</label>
						{#if unitClass(niUnit) === 'time'}
							<label class="block">
								<span class="mb-0.5 block text-xs text-gray-500">Start</span>
								<input
									type="time"
									bind:value={niStart}
									oninput={onDraftTime}
									class="rounded border border-gray-300 px-2 py-1 text-sm"
								/>
							</label>
							<label class="block">
								<span class="mb-0.5 block text-xs text-gray-500">End</span>
								<input
									type="time"
									bind:value={niEnd}
									oninput={onDraftTime}
									class="rounded border border-gray-300 px-2 py-1 text-sm"
								/>
							</label>
						{/if}
						<label class="block">
							<span class="mb-0.5 block text-xs text-gray-500">Qty</span>
							<input
								type="number"
								step="any"
								min="0"
								bind:value={niQuantity}
								class="w-20 rounded border border-gray-300 px-2 py-1 text-sm"
							/>
						</label>
						{#if niCode.trim() === ''}
							<label class="block">
								<span class="mb-0.5 block text-xs text-gray-500">Unit price</span>
								<input
									type="number"
									step="any"
									min="0"
									bind:value={niUnitPrice}
									class="w-24 rounded border border-gray-300 px-2 py-1 text-sm"
								/>
							</label>
						{/if}
						<button
							type="button"
							onclick={addItem}
							disabled={itemsBusy}
							class="rounded border border-gray-300 px-3 py-1 text-sm hover:bg-white disabled:opacity-50"
						>
							Add item
						</button>
					</div>
					{#if niCode.trim() !== ''}
						<p class="text-xs text-gray-400">Coded lines are priced from the NDIS catalogue on save.</p>
					{/if}
				</div>
			</div>
		{:else}
			<p class="text-xs text-gray-400">Save the shift first, then re-open it to add line items.</p>
		{/if}

		{#if error}
			<p class="text-sm text-red-600">{error}</p>
		{/if}

		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={() => (inline ? oncancel?.() : (open = false))}
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
{/snippet}

{#if inline}
	<div class="rounded border border-gray-200 bg-white p-4">
		<h2 class="mb-3 text-lg font-semibold">{title}</h2>
		{@render body()}
	</div>
{:else}
	<Modal bind:open {title}>
		{@render body()}
	</Modal>
{/if}
