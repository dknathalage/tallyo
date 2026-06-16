<script lang="ts">
	import { onMount } from 'svelte';
	import { recurring } from '$lib/stores/recurring.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import { apiPost } from '$lib/api/client';
	import type { RecurringTemplate, RecurringFrequency, RecurringLine, Invoice } from '$lib/api/types';

	// A draft line-item row used by the editor.
	interface LineRow {
		code: string;
		description: string;
		unit: string;
		quantity: number;
		unitPrice: number;
		gstFree: boolean;
	}

	const FREQUENCIES: RecurringFrequency[] = ['weekly', 'monthly', 'quarterly'];

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	// Client-side search.
	let search = $state('');
	const filtered = $derived.by<RecurringTemplate[]>(() => {
		const q = search.trim().toLowerCase();
		if (q === '') return recurring.items;
		return recurring.items.filter(
			(t) => t.name.toLowerCase().includes(q) || t.participantName.toLowerCase().includes(q)
		);
	});

	// Form state (shared by create + edit).
	let showForm = $state(false);
	let editId = $state<number | null>(null);
	let formName = $state('');
	let formParticipantId = $state('');
	let formFrequency = $state<RecurringFrequency>('monthly');
	let formNextDue = $state('');
	let formTaxRateId = $state('');
	let formNotes = $state('');
	let formActive = $state(true);
	let lines = $state<LineRow[]>([]);
	let saving = $state(false);
	let formError = $state<string | null>(null);
	let rowError = $state<string | null>(null);
	let message = $state<string | null>(null);
	let busy = $state(false);

	// Selected tax-rate percent (recurring stores only a number, no taxRateId).
	const previewTaxRate = $derived.by<number>(() => {
		if (formTaxRateId === '') return 0;
		const id = Number(formTaxRateId);
		const tr = taxRates.items.find((t) => t.id === id);
		return tr ? tr.rate : 0;
	});

	onMount(() => {
		recurring.ensureSubscribed();
		void recurring.load();
		participants.ensureSubscribed();
		void participants.load();
		taxRates.ensureSubscribed();
		void taxRates.load();
	});

	function lineAmount(row: LineRow): number {
		return (Number(row.quantity) || 0) * (Number(row.unitPrice) || 0);
	}

	function newLine(): LineRow {
		return { code: '', description: '', unit: '', quantity: 1, unitPrice: 0, gstFree: true };
	}

	function addLine(): void {
		lines.push(newLine());
	}

	function removeLine(index: number): void {
		lines.splice(index, 1);
	}

	function resetForm(): void {
		showForm = false;
		editId = null;
		formName = '';
		formParticipantId = '';
		formFrequency = 'monthly';
		formNextDue = '';
		formTaxRateId = '';
		formNotes = '';
		formActive = true;
		lines = [];
		formError = null;
	}

	function openCreate(): void {
		resetForm();
		formNextDue = new Date().toISOString().slice(0, 10);
		lines = [newLine()];
		showForm = true;
	}

	function buildPayload() {
		const items: RecurringLine[] = lines.map((row, i) => ({
			supportItemId: null,
			customItemId: null,
			code: row.code,
			description: row.description,
			unit: row.unit,
			quantity: Number(row.quantity),
			unitPrice: Number(row.unitPrice),
			gstFree: row.gstFree,
			sortOrder: i
		}));
		return {
			participantId: formParticipantId === '' ? null : Number(formParticipantId),
			planManagerId: null,
			name: formName,
			frequency: formFrequency,
			nextDue: formNextDue,
			lineItems: items,
			taxRate: previewTaxRate,
			notes: formNotes,
			isActive: formActive
		};
	}

	async function submitForm(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		if (formParticipantId === '') {
			formError = 'Please select a participant.';
			return;
		}
		saving = true;
		try {
			const payload = buildPayload();
			if (editId === null) {
				await recurring.crud.create(payload);
			} else {
				await recurring.crud.update(editId, payload);
			}
			resetForm();
			await recurring.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to save template.';
		} finally {
			saving = false;
		}
	}

	async function startEdit(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			const full = await recurring.crud.get(id);
			editId = full.id;
			formName = full.name;
			formParticipantId = full.participantId === null ? '' : String(full.participantId);
			formFrequency = full.frequency;
			formNextDue = full.nextDue ? full.nextDue.slice(0, 10) : '';
			const matched = taxRates.items.find((t) => t.rate === full.taxRate);
			formTaxRateId = matched ? String(matched.id) : '';
			formNotes = full.notes;
			formActive = full.isActive;
			lines = full.lineItems
				.slice()
				.sort((a, b) => a.sortOrder - b.sortOrder)
				.map((li) => ({
					code: li.code,
					description: li.description,
					unit: li.unit,
					quantity: li.quantity,
					unitPrice: li.unitPrice,
					gstFree: li.gstFree
				}));
			if (lines.length === 0) {
				lines = [newLine()];
			}
			formError = null;
			showForm = true;
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to load template.';
		} finally {
			busy = false;
		}
	}

	async function remove(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await recurring.crud.remove(id);
			await recurring.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete template.';
		} finally {
			busy = false;
		}
	}

	async function generateNow(id: number): Promise<void> {
		rowError = null;
		message = null;
		busy = true;
		try {
			const inv = await apiPost<Invoice>('/api/recurring/' + id + '/generate', {});
			message = inv !== null ? 'Generated invoice ' + inv.number : 'Generated invoice.';
			await recurring.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to generate invoice.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<div class="mb-6 flex items-center justify-between">
			<div>
				<h1 class="mb-1 text-xl font-semibold">Recurring templates</h1>
				<p class="text-sm text-gray-500">Schedule invoices that generate on a recurring cadence.</p>
			</div>
			<button
				type="button"
				onclick={openCreate}
				class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
			>
				New template
			</button>
		</div>

		{#if showForm}
			<form class="mb-8 space-y-4 rounded border border-gray-200 bg-white p-4" onsubmit={submitForm}>
				<h2 class="text-base font-semibold">
					{editId === null ? 'New template' : 'Edit template'}
				</h2>

				<div class="grid grid-cols-2 gap-3">
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Name</span>
						<input
							type="text"
							bind:value={formName}
							required
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						/>
					</label>
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Participant</span>
						<select
							bind:value={formParticipantId}
							required
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						>
							<option value="">— select —</option>
							{#each participants.items as p (p.id)}
								<option value={String(p.id)}>{p.name}</option>
							{/each}
						</select>
					</label>
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Frequency</span>
						<select
							bind:value={formFrequency}
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm capitalize"
						>
							{#each FREQUENCIES as f (f)}
								<option value={f}>{f}</option>
							{/each}
						</select>
					</label>
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Next due</span>
						<input
							type="date"
							bind:value={formNextDue}
							required
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						/>
					</label>
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Tax rate</span>
						<select
							bind:value={formTaxRateId}
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						>
							<option value="">— none —</option>
							{#each taxRates.items as tr (tr.id)}
								<option value={String(tr.id)}>{tr.name} ({tr.rate}%)</option>
							{/each}
						</select>
					</label>
					<label class="col-span-1 flex items-end gap-2">
						<input type="checkbox" bind:checked={formActive} class="h-4 w-4" />
						<span class="text-sm font-medium">Active</span>
					</label>
					<label class="col-span-2">
						<span class="mb-1 block text-sm font-medium">Notes</span>
						<input
							type="text"
							bind:value={formNotes}
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						/>
					</label>
				</div>

				<div>
					<div class="mb-2 flex items-center justify-between">
						<span class="text-sm font-medium">Line items</span>
						<button
							type="button"
							onclick={addLine}
							class="rounded border border-gray-300 px-3 py-1 text-sm hover:bg-gray-50"
						>
							Add line
						</button>
					</div>

					<div class="overflow-hidden rounded border border-gray-200">
						<table class="w-full text-sm">
							<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
								<tr>
									<th class="w-40 px-3 py-2 font-medium">Code</th>
									<th class="px-3 py-2 font-medium">Description</th>
									<th class="w-20 px-3 py-2 font-medium">Qty</th>
									<th class="w-28 px-3 py-2 font-medium">Unit price</th>
									<th class="w-16 px-3 py-2 font-medium">GST-free</th>
									<th class="w-24 px-3 py-2 font-medium text-right">Amount</th>
									<th class="w-12 px-3 py-2"></th>
								</tr>
							</thead>
							<tbody>
								{#each lines as line, i (i)}
									<tr class="border-b border-gray-100 last:border-0">
										<td class="px-3 py-2">
											<input
												type="text"
												bind:value={line.code}
												placeholder="NDIS code"
												class="w-full rounded border border-gray-300 px-2 py-1 font-mono text-xs"
											/>
										</td>
										<td class="px-3 py-2">
											<input
												type="text"
												bind:value={line.description}
												class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
											/>
										</td>
										<td class="px-3 py-2">
											<input
												type="number"
												step="any"
												bind:value={line.quantity}
												class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
											/>
										</td>
										<td class="px-3 py-2">
											<input
												type="number"
												step="any"
												bind:value={line.unitPrice}
												class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
											/>
										</td>
										<td class="px-3 py-2 text-center">
											<input type="checkbox" bind:checked={line.gstFree} class="h-4 w-4" />
										</td>
										<td class="px-3 py-2 text-right whitespace-nowrap">
											{money(lineAmount(line))}
										</td>
										<td class="px-3 py-2 text-right">
											<button
												type="button"
												onclick={() => removeLine(i)}
												class="text-red-600 hover:underline"
												aria-label="Remove line"
											>
												✕
											</button>
										</td>
									</tr>
								{:else}
									<tr>
										<td colspan="7" class="px-3 py-4 text-center text-gray-500"> No line items. </td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>

				{#if formError}
					<p class="text-sm text-red-600">{formError}</p>
				{/if}

				<div class="flex gap-2">
					<button
						type="submit"
						disabled={saving}
						class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
					>
						{saving ? 'Saving…' : editId === null ? 'Create template' : 'Save changes'}
					</button>
					<button
						type="button"
						onclick={resetForm}
						class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
					>
						Cancel
					</button>
				</div>
			</form>
		{/if}
	</section>

	<section>
		<label class="mb-4 block max-w-sm">
			<span class="mb-1 block text-sm font-medium">Search</span>
			<input
				type="text"
				bind:value={search}
				placeholder="Filter by name or participant"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>

		{#if recurring.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if recurring.error}
			<p class="text-sm text-red-600">{recurring.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}
		{#if message}
			<p class="mb-3 text-sm text-green-700">{message}</p>
		{/if}

		<div class="overflow-hidden rounded border border-gray-200 bg-white">
			<table class="w-full text-sm">
				<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
					<tr>
						<th class="px-3 py-2 font-medium">Name</th>
						<th class="px-3 py-2 font-medium">Participant</th>
						<th class="px-3 py-2 font-medium">Frequency</th>
						<th class="px-3 py-2 font-medium">Next due</th>
						<th class="px-3 py-2 font-medium">Status</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as t (t.id)}
						<tr class="border-b border-gray-100 last:border-0">
							<td class="px-3 py-2 font-medium">{t.name}</td>
							<td class="px-3 py-2 text-gray-600">{t.participantName || '—'}</td>
							<td class="px-3 py-2 text-gray-600 capitalize">{t.frequency}</td>
							<td class="px-3 py-2 text-gray-600">
								{t.nextDue ? t.nextDue.slice(0, 10) : '—'}
							</td>
							<td class="px-3 py-2">
								<span
									class="inline-block rounded px-2 py-0.5 text-xs font-medium {t.isActive
										? 'bg-green-100 text-green-800'
										: 'bg-gray-100 text-gray-700'}"
								>
									{t.isActive ? 'Active' : 'Inactive'}
								</span>
							</td>
							<td class="px-3 py-2 text-right whitespace-nowrap">
								<button
									type="button"
									onclick={() => generateNow(t.id)}
									disabled={busy}
									class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
								>
									Generate now
								</button>
								<button
									type="button"
									onclick={() => startEdit(t.id)}
									disabled={busy}
									class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
								>
									Edit
								</button>
								<button
									type="button"
									onclick={() => remove(t.id)}
									disabled={busy}
									class="text-red-600 hover:underline disabled:opacity-50"
								>
									Delete
								</button>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="6" class="px-3 py-6 text-center text-gray-500">
								No recurring templates found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
