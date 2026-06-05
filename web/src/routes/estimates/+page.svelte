<script lang="ts">
	import { onMount } from 'svelte';
	import { estimates } from '$lib/stores/estimates.svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import { apiPost } from '$lib/api/client';
	import type { Estimate, EstimateStatus, EstimateLineItemInput } from '$lib/api/types';

	// A draft line-item row used by the editor.
	interface LineRow {
		description: string;
		quantity: number;
		rate: number;
		notes: string;
	}

	// Result of the convert-to-invoice endpoint.
	interface ConvertResult {
		invoiceId: number;
		invoiceNumber: string;
		estimateNumber: string;
	}

	const STATUSES: EstimateStatus[] = ['draft', 'accepted', 'declined', 'converted'];

	// Selects bind to a string id ('' means none); convert to number | null.
	function toNullableId(v: string): number | null {
		return v === '' ? null : Number(v);
	}

	function statusClass(status: string): string {
		switch (status) {
			case 'accepted':
				return 'bg-green-100 text-green-800';
			case 'converted':
				return 'bg-blue-100 text-blue-800';
			case 'declined':
				return 'bg-red-100 text-red-800';
			default:
				return 'bg-gray-100 text-gray-700';
		}
	}

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	// List filter.
	let selectedStatus = $state<'all' | EstimateStatus>('all');
	const filtered = $derived.by<Estimate[]>(() => {
		if (selectedStatus === 'all') return estimates.items;
		return estimates.items.filter((est) => est.status === selectedStatus);
	});

	// Form state (shared by create + edit).
	let showForm = $state(false);
	let editId = $state<number | null>(null);
	let formClientId = $state('');
	let formTaxRateId = $state('');
	let formDate = $state('');
	let formValidUntil = $state('');
	let formNotes = $state('');
	let lines = $state<LineRow[]>([]);
	let saving = $state(false);
	let formError = $state<string | null>(null);
	let rowError = $state<string | null>(null);
	let convertMsg = $state<string | null>(null);
	let busy = $state(false);

	// Live totals preview. Server remains authoritative on save.
	const subtotal = $derived.by<number>(() => {
		let sum = 0;
		for (let i = 0; i < lines.length; i++) {
			const q = Number(lines[i].quantity) || 0;
			const r = Number(lines[i].rate) || 0;
			sum += q * r;
		}
		return sum;
	});
	const previewTaxRate = $derived.by<number>(() => {
		if (formTaxRateId === '') return 0;
		const id = Number(formTaxRateId);
		const tr = taxRates.items.find((t) => t.id === id);
		return tr ? tr.rate : 0;
	});
	const taxAmount = $derived(subtotal * (previewTaxRate / 100));
	const total = $derived(subtotal + taxAmount);

	onMount(() => {
		estimates.ensureSubscribed();
		void estimates.load();
		clients.ensureSubscribed();
		void clients.load();
		taxRates.ensureSubscribed();
		void taxRates.load();
	});

	function lineAmount(row: LineRow): number {
		return (Number(row.quantity) || 0) * (Number(row.rate) || 0);
	}

	function addLine(): void {
		lines.push({ description: '', quantity: 1, rate: 0, notes: '' });
	}

	function removeLine(index: number): void {
		lines.splice(index, 1);
	}

	function resetForm(): void {
		showForm = false;
		editId = null;
		formClientId = '';
		formTaxRateId = '';
		formDate = '';
		formValidUntil = '';
		formNotes = '';
		lines = [];
		formError = null;
	}

	function openCreate(): void {
		resetForm();
		const today = new Date().toISOString().slice(0, 10);
		formDate = today;
		formValidUntil = today;
		lines = [{ description: '', quantity: 1, rate: 0, notes: '' }];
		showForm = true;
	}

	function buildPayload() {
		const items: EstimateLineItemInput[] = lines.map((row, i) => ({
			description: row.description,
			quantity: Number(row.quantity),
			rate: Number(row.rate),
			notes: row.notes ?? '',
			sortOrder: i
		}));
		return {
			clientId: Number(formClientId),
			date: formDate,
			validUntil: formValidUntil,
			taxRate: previewTaxRate,
			taxRateId: toNullableId(formTaxRateId),
			notes: formNotes,
			status: 'draft' as EstimateStatus,
			currencyCode: 'USD',
			lineItems: items
		};
	}

	async function submitForm(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		if (formClientId === '') {
			formError = 'Please select a client.';
			return;
		}
		saving = true;
		try {
			const payload = buildPayload();
			if (editId === null) {
				await estimates.crud.create(payload);
			} else {
				await estimates.crud.update(editId, payload);
			}
			resetForm();
			await estimates.load();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to save estimate.';
		} finally {
			saving = false;
		}
	}

	async function startEdit(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			const full = await estimates.crud.get(id);
			editId = full.id;
			formClientId = String(full.clientId);
			formTaxRateId = full.taxRateId === null ? '' : String(full.taxRateId);
			formDate = full.date ? full.date.slice(0, 10) : '';
			formValidUntil = full.validUntil ? full.validUntil.slice(0, 10) : '';
			formNotes = full.notes;
			lines = full.lineItems
				.slice()
				.sort((a, b) => a.sortOrder - b.sortOrder)
				.map((li) => ({
					description: li.description,
					quantity: li.quantity,
					rate: li.rate,
					notes: li.notes
				}));
			if (lines.length === 0) {
				lines = [{ description: '', quantity: 1, rate: 0, notes: '' }];
			}
			formError = null;
			showForm = true;
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to load estimate.';
		} finally {
			busy = false;
		}
	}

	async function changeStatus(id: number, status: string): Promise<void> {
		if (status === '') return;
		rowError = null;
		busy = true;
		try {
			await apiPost('/api/estimates/' + id + '/status', { status });
			await estimates.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update status.';
		} finally {
			busy = false;
		}
	}

	async function duplicate(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await apiPost('/api/estimates/' + id + '/duplicate', {});
			await estimates.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to duplicate estimate.';
		} finally {
			busy = false;
		}
	}

	async function convert(id: number): Promise<void> {
		rowError = null;
		convertMsg = null;
		busy = true;
		try {
			const result = await apiPost<ConvertResult>('/api/estimates/' + id + '/convert', {});
			if (result !== null) {
				convertMsg = `Converted to invoice ${result.invoiceNumber}.`;
			}
			await estimates.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to convert estimate.';
		} finally {
			busy = false;
		}
	}

	async function remove(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await estimates.crud.remove(id);
			await estimates.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete estimate.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-8">
	<section>
		<div class="mb-6 flex items-center justify-between">
			<div>
				<h1 class="mb-1 text-xl font-semibold">Estimates</h1>
				<p class="text-sm text-gray-500">Create and manage estimates with line items.</p>
			</div>
			<button
				type="button"
				onclick={openCreate}
				class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
			>
				New estimate
			</button>
		</div>

		{#if showForm}
			<form
				class="mb-8 space-y-4 rounded border border-gray-200 bg-white p-4"
				onsubmit={submitForm}
			>
				<h2 class="text-base font-semibold">
					{editId === null ? 'New estimate' : 'Edit estimate'}
				</h2>

				<div class="grid grid-cols-2 gap-3">
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Client</span>
						<select
							bind:value={formClientId}
							required
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						>
							<option value="">— select —</option>
							{#each clients.items as client (client.id)}
								<option value={String(client.id)}>{client.name}</option>
							{/each}
						</select>
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
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Date</span>
						<input
							type="date"
							bind:value={formDate}
							required
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						/>
					</label>
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Valid Until</span>
						<input
							type="date"
							bind:value={formValidUntil}
							required
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						/>
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
									<th class="px-3 py-2 font-medium">Description</th>
									<th class="w-24 px-3 py-2 font-medium">Qty</th>
									<th class="w-28 px-3 py-2 font-medium">Rate</th>
									<th class="w-28 px-3 py-2 font-medium text-right">Amount</th>
									<th class="w-12 px-3 py-2"></th>
								</tr>
							</thead>
							<tbody>
								{#each lines as line, i (i)}
									<tr class="border-b border-gray-100 last:border-0">
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
												bind:value={line.rate}
												class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
											/>
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
										<td colspan="5" class="px-3 py-4 text-center text-gray-500">
											No line items.
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>

				<div class="flex justify-end">
					<dl class="w-56 space-y-1 text-sm">
						<div class="flex justify-between">
							<dt class="text-gray-500">Subtotal</dt>
							<dd>{money(subtotal)}</dd>
						</div>
						<div class="flex justify-between">
							<dt class="text-gray-500">Tax ({previewTaxRate}%)</dt>
							<dd>{money(taxAmount)}</dd>
						</div>
						<div class="flex justify-between border-t border-gray-200 pt-1 font-semibold">
							<dt>Total</dt>
							<dd>{money(total)}</dd>
						</div>
					</dl>
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
						{saving ? 'Saving…' : editId === null ? 'Create estimate' : 'Save changes'}
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
		<div class="mb-4 flex items-center gap-2">
			<span class="text-sm font-medium">Filter:</span>
			<button
				type="button"
				onclick={() => (selectedStatus = 'all')}
				class="rounded px-3 py-1 text-sm {selectedStatus === 'all'
					? 'bg-gray-900 text-white'
					: 'border border-gray-300 hover:bg-gray-50'}"
			>
				All
			</button>
			{#each STATUSES as s (s)}
				<button
					type="button"
					onclick={() => (selectedStatus = s)}
					class="rounded px-3 py-1 text-sm capitalize {selectedStatus === s
						? 'bg-gray-900 text-white'
						: 'border border-gray-300 hover:bg-gray-50'}"
				>
					{s}
				</button>
			{/each}
		</div>

		{#if estimates.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if estimates.error}
			<p class="text-sm text-red-600">{estimates.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}
		{#if convertMsg}
			<p class="mb-3 text-sm text-green-700">{convertMsg}</p>
		{/if}

		<div class="overflow-hidden rounded border border-gray-200 bg-white">
			<table class="w-full text-sm">
				<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
					<tr>
						<th class="px-3 py-2 font-medium">Number</th>
						<th class="px-3 py-2 font-medium">Client</th>
						<th class="px-3 py-2 font-medium">Date</th>
						<th class="px-3 py-2 font-medium text-right">Total</th>
						<th class="px-3 py-2 font-medium">Status</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as est (est.id)}
						<tr class="border-b border-gray-100 last:border-0">
							<td class="px-3 py-2 font-medium">{est.estimateNumber}</td>
							<td class="px-3 py-2 text-gray-600">{est.clientName || '—'}</td>
							<td class="px-3 py-2 text-gray-600">{est.date ? est.date.slice(0, 10) : '—'}</td>
							<td class="px-3 py-2 text-right">{money(est.total)}</td>
							<td class="px-3 py-2">
								<span
									class="inline-block rounded px-2 py-0.5 text-xs font-medium capitalize {statusClass(
										est.status
									)}"
								>
									{est.status}
								</span>
							</td>
							<td class="px-3 py-2 text-right whitespace-nowrap">
								<select
									value={est.status}
									disabled={busy}
									onchange={(e) => changeStatus(est.id, e.currentTarget.value)}
									class="mr-2 rounded border border-gray-300 px-1 py-1 text-xs disabled:opacity-50"
									aria-label="Change status"
								>
									{#each STATUSES as s (s)}
										<option value={s}>{s}</option>
									{/each}
								</select>
								<a
									href={'/api/estimates/' + est.id + '/pdf'}
									target="_blank"
									rel="noopener"
									class="mr-2 text-blue-600 hover:underline"
								>
									PDF
								</a>
								{#if est.status === 'accepted' && !est.convertedInvoiceId}
									<button
										type="button"
										onclick={() => convert(est.id)}
										disabled={busy}
										class="mr-2 text-blue-700 hover:underline disabled:opacity-50"
									>
										Convert
									</button>
								{/if}
								<button
									type="button"
									onclick={() => startEdit(est.id)}
									disabled={busy}
									class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
								>
									Edit
								</button>
								<button
									type="button"
									onclick={() => duplicate(est.id)}
									disabled={busy}
									class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
								>
									Duplicate
								</button>
								<button
									type="button"
									onclick={() => remove(est.id)}
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
								No estimates found.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
