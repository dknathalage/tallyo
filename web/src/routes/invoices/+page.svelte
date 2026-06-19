<script lang="ts">
	import { onMount } from 'svelte';
	import { invoices } from '$lib/stores/invoices.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import { apiGet, apiPost, apiDelete, ApiError } from '$lib/api/client';
	import LineItemsEditor from '$lib/components/LineItemsEditor.svelte';
	import type { EditorLine } from '$lib/components/LineItemsEditor.svelte';
	import type {
		Invoice,
		InvoiceStatus,
		LineItemInput,
		Payment,
		ValidationDetail
	} from '$lib/api/types';

	const STATUSES: InvoiceStatus[] = ['draft', 'sent', 'overdue', 'paid'];

	function statusClass(status: string): string {
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

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	// List filter.
	let selectedStatus = $state<'all' | InvoiceStatus>('all');
	const filtered = $derived.by<Invoice[]>(() => {
		if (selectedStatus === 'all') return invoices.items;
		return invoices.items.filter((inv) => inv.status === selectedStatus);
	});

	// Form state (shared by create + edit).
	let showForm = $state(false);
	let editId = $state<number | null>(null);
	let formParticipantId = $state('');
	let formIssueDate = $state('');
	let formDueDate = $state('');
	let formNotes = $state('');
	let lines = $state<EditorLine[]>([]);
	let validationDetails = $state<ValidationDetail[]>([]);
	let saving = $state(false);
	let formError = $state<string | null>(null);
	let rowError = $state<string | null>(null);
	let busy = $state(false);

	// Live subtotal preview (sum of line amounts). Server is authoritative on
	// tax + total, so we only preview the subtotal here.
	const subtotalPreview = $derived.by<number>(() => {
		let sum = 0;
		for (let i = 0; i < lines.length; i++) {
			const q = Number(lines[i].quantity) || 0;
			const r = Number(lines[i].unitPrice) || 0;
			sum += q * r;
		}
		return sum;
	});

	onMount(() => {
		invoices.ensureSubscribed();
		void invoices.load();
		participants.ensureSubscribed();
		void participants.load();
		customItems.ensureSubscribed();
		void customItems.load();
		businessProfile.subscribe();
		void businessProfile.load();
	});

	function resetForm(): void {
		showForm = false;
		editId = null;
		formParticipantId = '';
		formIssueDate = '';
		formDueDate = '';
		formNotes = '';
		lines = [];
		validationDetails = [];
		formError = null;
	}

	function openCreate(): void {
		resetForm();
		const today = new Date().toISOString().slice(0, 10);
		formIssueDate = today;
		formDueDate = today;
		lines = [
			{
				kind: 'support',
				customItemId: null,
				catalogVersionId: null,
				code: '',
				description: '',
				serviceDate: today,
				unit: '',
				quantity: 1,
				unitPrice: 0,
				gstFree: true,
				sortOrder: 0
			}
		];
		showForm = true;
	}

	function buildPayload() {
		const items: LineItemInput[] = lines.map((row, i) => ({
			supportItemId: null,
			customItemId: row.kind === 'custom' ? row.customItemId : null,
			// Carry the pinned version for existing support lines so the server
			// re-validates against their original catalogue version (frozen price);
			// new lines have null → priced from the current version.
			catalogVersionId: row.kind === 'support' ? row.catalogVersionId : null,
			code: row.kind === 'support' ? row.code : '',
			description: row.description,
			serviceDate: row.serviceDate,
			unit: row.unit,
			startTime: '',
			endTime: '',
			quantity: Number(row.quantity),
			unitPrice: Number(row.unitPrice),
			gstFree: row.gstFree,
			sortOrder: i
		}));
		return {
			participantId: Number(formParticipantId),
			planManagerId: null,
			status: 'draft' as InvoiceStatus,
			issueDate: formIssueDate,
			dueDate: formDueDate,
			notes: formNotes,
			lineItems: items
		};
	}

	async function submitForm(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		validationDetails = [];
		if (formParticipantId === '') {
			formError = 'Please select a participant.';
			return;
		}
		if (lines.length === 0) {
			formError = 'Add at least one line item.';
			return;
		}
		saving = true;
		try {
			const payload = buildPayload();
			if (editId === null) {
				await invoices.crud.create(payload);
			} else {
				await invoices.crud.update(editId, payload);
			}
			resetForm();
			await invoices.load();
		} catch (err) {
			if (err instanceof ApiError && err.status === 422) {
				validationDetails = err.details;
				formError = 'Please fix the highlighted line items.';
			} else {
				formError = err instanceof Error ? err.message : 'Failed to save invoice.';
			}
		} finally {
			saving = false;
		}
	}

	async function startEdit(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			const full = await invoices.crud.get(id);
			editId = full.id;
			formParticipantId = String(full.participantId);
			formIssueDate = full.issueDate ? full.issueDate.slice(0, 10) : '';
			formDueDate = full.dueDate ? full.dueDate.slice(0, 10) : '';
			formNotes = full.notes;
			validationDetails = [];
			lines = full.lineItems
				.slice()
				.sort((a, b) => a.sortOrder - b.sortOrder)
				.map((li) => ({
					kind: (li.customItemId === null && li.code !== '' ? 'support' : 'custom') as
						| 'support'
						| 'custom',
					customItemId: li.customItemId,
					catalogVersionId: li.catalogVersionId,
					code: li.code,
					description: li.description,
					serviceDate: li.serviceDate ? li.serviceDate.slice(0, 10) : '',
					unit: li.unit,
					quantity: li.quantity,
					unitPrice: li.unitPrice,
					gstFree: li.gstFree,
					sortOrder: li.sortOrder
				}));
			if (lines.length === 0) {
				lines = [
					{
						kind: 'support',
						customItemId: null,
						catalogVersionId: null,
						code: '',
						description: '',
						serviceDate: new Date().toISOString().slice(0, 10),
						unit: '',
						quantity: 1,
						unitPrice: 0,
						gstFree: true,
						sortOrder: 0
					}
				];
			}
			formError = null;
			showForm = true;
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to load invoice.';
		} finally {
			busy = false;
		}
	}

	async function changeStatus(id: number, status: string): Promise<void> {
		if (status === '') return;
		rowError = null;
		busy = true;
		try {
			await apiPost('/api/invoices/' + id + '/status', { status });
			await invoices.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to update status.';
		} finally {
			busy = false;
		}
	}

	async function remove(id: number): Promise<void> {
		rowError = null;
		busy = true;
		try {
			await invoices.crud.remove(id);
			await invoices.load();
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to delete invoice.';
		} finally {
			busy = false;
		}
	}

	// Payments panel: one invoice expanded at a time.
	let paymentsInvoiceId = $state<number | null>(null);
	let paymentsList = $state<Payment[]>([]);
	let paymentsLoading = $state(false);
	let paymentsError = $state<string | null>(null);
	let payAmount = $state('');
	let payDate = $state('');
	let payMethod = $state('');
	let payNotes = $state('');
	let paySaving = $state(false);

	const paymentsPaid = $derived.by<number>(() => {
		let sum = 0;
		for (let i = 0; i < paymentsList.length; i++) {
			sum += Number(paymentsList[i].amount) || 0;
		}
		return sum;
	});

	function paymentBadge(total: number, paid: number): { label: string; cls: string } {
		const t = Number.isFinite(total) ? total : 0;
		const p = Number.isFinite(paid) ? paid : 0;
		if (p >= t && t > 0) return { label: 'Paid', cls: 'bg-green-100 text-green-800' };
		if (p > 0) return { label: 'Partial', cls: 'bg-amber-100 text-amber-800' };
		return { label: 'Unpaid', cls: 'bg-gray-100 text-gray-700' };
	}

	async function loadPayments(id: number): Promise<void> {
		paymentsLoading = true;
		paymentsError = null;
		try {
			const list = await apiGet<Payment[]>('/api/invoices/' + id + '/payments');
			paymentsList = list ?? [];
		} catch (err) {
			paymentsError = err instanceof Error ? err.message : 'Failed to load payments.';
			paymentsList = [];
		} finally {
			paymentsLoading = false;
		}
	}

	async function togglePayments(id: number): Promise<void> {
		if (paymentsInvoiceId === id) {
			paymentsInvoiceId = null;
			paymentsList = [];
			return;
		}
		paymentsInvoiceId = id;
		paymentsList = [];
		payAmount = '';
		payDate = new Date().toISOString().slice(0, 10);
		payMethod = '';
		payNotes = '';
		paymentsError = null;
		await loadPayments(id);
	}

	async function recordPayment(e: SubmitEvent, id: number): Promise<void> {
		e.preventDefault();
		paymentsError = null;
		const amount = Number(payAmount);
		if (!Number.isFinite(amount) || amount <= 0) {
			paymentsError = 'Amount must be greater than 0.';
			return;
		}
		paySaving = true;
		try {
			await apiPost('/api/invoices/' + id + '/payments', {
				amount,
				paymentDate: payDate,
				method: payMethod,
				notes: payNotes
			});
			payAmount = '';
			payMethod = '';
			payNotes = '';
			await loadPayments(id);
			await invoices.load();
		} catch (err) {
			paymentsError = err instanceof Error ? err.message : 'Failed to record payment.';
		} finally {
			paySaving = false;
		}
	}

	async function deletePayment(pid: number, id: number): Promise<void> {
		paymentsError = null;
		try {
			await apiDelete('/api/payments/' + pid);
			await loadPayments(id);
			await invoices.load();
		} catch (err) {
			paymentsError = err instanceof Error ? err.message : 'Failed to delete payment.';
		}
	}
</script>

<div class="space-y-8">
	<section>
		<div class="mb-6 flex items-center justify-between">
			<div>
				<h1 class="mb-1 text-xl font-semibold">Invoices</h1>
				<p class="text-sm text-gray-500">NDIS-compliant invoices with price-cap validation.</p>
			</div>
			<div class="flex items-center gap-2">
				<a
					href="/api/export/invoices"
					target="_blank"
					rel="noopener"
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
				>
					Export CSV
				</a>
				<button
					type="button"
					onclick={openCreate}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white"
				>
					New invoice
				</button>
			</div>
		</div>

		{#if showForm}
			<form class="mb-8 space-y-4 rounded border border-gray-200 bg-white p-4" onsubmit={submitForm}>
				<h2 class="text-base font-semibold">
					{editId === null ? 'New invoice' : 'Edit invoice'}
				</h2>

				<div class="grid grid-cols-2 gap-3">
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
					<div class="col-span-1"></div>
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Issue date</span>
						<input
							type="date"
							bind:value={formIssueDate}
							required
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						/>
					</label>
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Due date</span>
						<input
							type="date"
							bind:value={formDueDate}
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

				<LineItemsEditor bind:lines details={validationDetails} />

				<div class="flex justify-end">
					<dl class="w-56 space-y-1 text-sm">
						<div class="flex justify-between">
							<dt class="text-gray-500">Subtotal (preview)</dt>
							<dd>{money(subtotalPreview)}</dd>
						</div>
						<p class="text-xs text-gray-400">Tax + total are calculated on save.</p>
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
						{saving ? 'Saving…' : editId === null ? 'Create invoice' : 'Save changes'}
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
		<div class="mb-4 flex flex-wrap items-center gap-2">
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

		{#if invoices.loading}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
		{#if invoices.error}
			<p class="text-sm text-red-600">{invoices.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}

		<div class="overflow-hidden rounded border border-gray-200 bg-white">
			<table class="w-full text-sm">
				<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
					<tr>
						<th class="px-3 py-2 font-medium">Number</th>
						<th class="px-3 py-2 font-medium">Participant</th>
						<th class="px-3 py-2 font-medium">Issued</th>
						<th class="px-3 py-2 font-medium text-right">Total</th>
						<th class="px-3 py-2 font-medium">Status</th>
						<th class="px-3 py-2 font-medium">Payment</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as inv (inv.id)}
						<tr class="border-b border-gray-100 last:border-0">
							<td class="px-3 py-2 font-medium">
									<a href={`/invoices/${inv.id}`} class="text-gray-900 hover:underline">
										{inv.number}
									</a>
								</td>
							<td class="px-3 py-2 text-gray-600">{inv.participantName || '—'}</td>
							<td class="px-3 py-2 text-gray-600">
								{inv.issueDate ? inv.issueDate.slice(0, 10) : '—'}
							</td>
							<td class="px-3 py-2 text-right">{money(inv.total)}</td>
							<td class="px-3 py-2">
								<span
									class="inline-block rounded px-2 py-0.5 text-xs font-medium capitalize {statusClass(
										inv.status
									)}"
								>
									{inv.status}
								</span>
							</td>
							<td class="px-3 py-2">
								<span
									class="inline-block rounded px-2 py-0.5 text-xs font-medium {paymentBadge(
										inv.total,
										paymentsInvoiceId === inv.id ? paymentsPaid : inv.totalPaid
									).cls}"
								>
									{paymentBadge(
										inv.total,
										paymentsInvoiceId === inv.id ? paymentsPaid : inv.totalPaid
									).label}
								</span>
							</td>
							<td class="px-3 py-2 text-right whitespace-nowrap">
								<select
									value={inv.status}
									disabled={busy}
									onchange={(e) => changeStatus(inv.id, e.currentTarget.value)}
									class="mr-2 rounded border border-gray-300 px-1 py-1 text-xs disabled:opacity-50"
									aria-label="Change status"
								>
									{#each STATUSES as s (s)}
										<option value={s}>{s}</option>
									{/each}
								</select>
								<a
									href={'/api/invoices/' + inv.id + '/pdf'}
									target="_blank"
									rel="noopener"
									class="mr-2 text-blue-600 hover:underline"
								>
									PDF
								</a>
								<button
									type="button"
									onclick={() => togglePayments(inv.id)}
									class="mr-2 text-gray-900 hover:underline"
								>
									{paymentsInvoiceId === inv.id ? 'Hide payments' : 'Payments'}
								</button>
								<button
									type="button"
									onclick={() => startEdit(inv.id)}
									disabled={busy}
									class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
								>
									Edit
								</button>
								<button
									type="button"
									onclick={() => remove(inv.id)}
									disabled={busy}
									class="text-red-600 hover:underline disabled:opacity-50"
								>
									Delete
								</button>
							</td>
						</tr>
						{#if paymentsInvoiceId === inv.id}
							<tr class="border-b border-gray-100 bg-gray-50">
								<td colspan="7" class="px-3 py-4">
									<div class="space-y-4">
										<div class="flex flex-wrap items-center gap-4 text-sm">
											<span class="font-semibold">Payments — {inv.number}</span>
											<span class="text-gray-500">Total: {money(inv.total)}</span>
											<span class="text-gray-500">Paid: {money(paymentsPaid)}</span>
											<span class="font-medium">Balance: {money(inv.total - paymentsPaid)}</span>
										</div>

										{#if paymentsError}
											<p class="text-sm text-red-600">{paymentsError}</p>
										{/if}

										<form
											class="flex flex-wrap items-end gap-2"
											onsubmit={(e) => recordPayment(e, inv.id)}
										>
											<label class="text-sm">
												<span class="mb-1 block font-medium">Amount</span>
												<input
													type="number"
													step="any"
													min="0"
													required
													bind:value={payAmount}
													class="w-28 rounded border border-gray-300 px-2 py-1 text-sm"
												/>
											</label>
											<label class="text-sm">
												<span class="mb-1 block font-medium">Date</span>
												<input
													type="date"
													bind:value={payDate}
													class="rounded border border-gray-300 px-2 py-1 text-sm"
												/>
											</label>
											<label class="text-sm">
												<span class="mb-1 block font-medium">Method</span>
												<input
													type="text"
													bind:value={payMethod}
													class="w-32 rounded border border-gray-300 px-2 py-1 text-sm"
												/>
											</label>
											<label class="text-sm">
												<span class="mb-1 block font-medium">Notes</span>
												<input
													type="text"
													bind:value={payNotes}
													class="w-40 rounded border border-gray-300 px-2 py-1 text-sm"
												/>
											</label>
											<button
												type="submit"
												disabled={paySaving}
												class="rounded bg-gray-900 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
											>
												{paySaving ? 'Saving…' : 'Record payment'}
											</button>
										</form>

										{#if paymentsLoading}
											<p class="text-sm text-gray-500">Loading payments…</p>
										{:else}
											<div class="overflow-hidden rounded border border-gray-200 bg-white">
												<table class="w-full text-sm">
													<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
														<tr>
															<th class="px-3 py-2 font-medium">Date</th>
															<th class="px-3 py-2 font-medium text-right">Amount</th>
															<th class="px-3 py-2 font-medium">Method</th>
															<th class="px-3 py-2 font-medium">Notes</th>
															<th class="w-12 px-3 py-2"></th>
														</tr>
													</thead>
													<tbody>
														{#each paymentsList as p (p.id)}
															<tr class="border-b border-gray-100 last:border-0">
																<td class="px-3 py-2 text-gray-600">
																	{p.paymentDate ? p.paymentDate.slice(0, 10) : '—'}
																</td>
																<td class="px-3 py-2 text-right">{money(p.amount)}</td>
																<td class="px-3 py-2 text-gray-600">{p.method || '—'}</td>
																<td class="px-3 py-2 text-gray-600">{p.notes || '—'}</td>
																<td class="px-3 py-2 text-right">
																	<button
																		type="button"
																		onclick={() => deletePayment(p.id, inv.id)}
																		class="text-red-600 hover:underline"
																		aria-label="Delete payment"
																	>
																		✕
																	</button>
																</td>
															</tr>
														{:else}
															<tr>
																<td colspan="5" class="px-3 py-4 text-center text-gray-500">
																	No payments recorded.
																</td>
															</tr>
														{/each}
													</tbody>
												</table>
											</div>
										{/if}
									</div>
								</td>
							</tr>
						{/if}
					{:else}
						<tr>
							<td colspan="7" class="px-3 py-6 text-center text-gray-500">No invoices found.</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
