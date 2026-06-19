<script lang="ts">
	import { onMount } from 'svelte';
	import { recurring } from '$lib/stores/recurring.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import { apiPost } from '$lib/api/client';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Trash2 from '@lucide/svelte/icons/trash-2';
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

	// Selected tax-rate percent (recurring stores only a number, no taxRateId).
	const previewTaxRate = $derived.by<number>(() => {
		if (formTaxRateId === '') return 0;
		const id = Number(formTaxRateId);
		const tr = taxRates.items.find((t) => t.id === id);
		return tr ? tr.rate : 0;
	});

	onMount(() => {
		recurring.ensureSubscribed();
		void recurring.query({ page: 1, limit: 50 });
		participants.ensureSubscribed();
		void participants.load();
		taxRates.ensureSubscribed();
		void taxRates.load();
	});

	// DataTable column definitions. Keys match RecurringTemplate JSON fields (and
	// the server allowlist), so one key drives filter, sort, and display.
	const columns: Column<RecurringTemplate>[] = [
		{ key: 'name', label: 'Name', sortable: true, filter: 'text' },
		{ key: 'participantName', label: 'Participant', sortable: true, filter: 'text' },
		{
			key: 'frequency',
			label: 'Frequency',
			sortable: true,
			filter: 'enum',
			values: ['weekly', 'monthly', 'quarterly']
		},
		{
			key: 'nextDue',
			label: 'Next due',
			sortable: true,
			filter: 'date',
			cell: (t) => (t.nextDue ? t.nextDue.slice(0, 10) : '—')
		},
		{
			key: 'taxRate',
			label: 'Tax rate',
			sortable: true,
			filter: 'number',
			cell: (t) => `${t.taxRate}%`
		},
		{
			// is_active is stored as 0/1; enum filter values are the integer strings.
			key: 'isActive',
			label: 'Status',
			sortable: true,
			filter: 'enum',
			values: ['1', '0'],
			cell: (t) => (t.isActive ? 'Active' : 'Inactive')
		}
	];

	// Selection-bar actions. Edit and Generate act on a single selected row (they
	// are inherently per-template); Delete handles any number of selected rows.
	const rowActions: RowAction<RecurringTemplate>[] = [
		{
			label: 'Edit',
			bulk: true,
			run: async (rows) => {
				if (rows.length !== 1) {
					rowError = 'Select exactly one template to edit.';
					return;
				}
				await startEdit(rows[0].id);
			}
		},
		{
			label: 'Generate now',
			bulk: true,
			run: async (rows) => {
				if (rows.length !== 1) {
					rowError = 'Select exactly one template to generate from.';
					return;
				}
				await generateNow(rows[0].id);
			}
		},
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await recurring.crud.remove(r.id); // bounded by selection
			}
		}
	];

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
			// The list refreshes via SSE invalidation (re-runs the active query).
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to save template.';
		} finally {
			saving = false;
		}
	}

	async function startEdit(id: number): Promise<void> {
		rowError = null;
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
		}
	}

	async function generateNow(id: number): Promise<void> {
		rowError = null;
		message = null;
		try {
			const inv = await apiPost<Invoice>('/api/recurring/' + id + '/generate', {});
			message = inv !== null ? 'Generated invoice ' + inv.number : 'Generated invoice.';
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to generate invoice.';
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
		{#if recurring.error}
			<p class="mb-3 text-sm text-red-600">{recurring.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}
		{#if message}
			<p class="mb-3 text-sm text-green-700">{message}</p>
		{/if}

		<DataTable
			title="Recurring"
			{columns}
			store={recurring}
			{rowActions}
			onNew={openCreate}
		/>
	</section>
</div>
