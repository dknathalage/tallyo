<script lang="ts">
	import { onMount } from 'svelte';
	import { estimates } from '$lib/stores/estimates.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import { apiPost, ApiError } from '$lib/api/client';
	import LineItemsEditor from '$lib/components/LineItemsEditor.svelte';
	import type { EditorLine } from '$lib/components/LineItemsEditor.svelte';
	import type {
		Estimate,
		EstimateStatus,
		EstimateLineItemInput,
		ValidationDetail
	} from '$lib/api/types';

	interface ConvertResult {
		invoiceId: number;
		invoiceNumber: string;
		estimateNumber: string;
	}

	const STATUSES: EstimateStatus[] = ['draft', 'accepted', 'declined', 'converted'];

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
	let formParticipantId = $state('');
	let formIssueDate = $state('');
	let formValidUntil = $state('');
	let formNotes = $state('');
	let lines = $state<EditorLine[]>([]);
	let validationDetails = $state<ValidationDetail[]>([]);
	let saving = $state(false);
	let formError = $state<string | null>(null);
	let rowError = $state<string | null>(null);
	let convertMsg = $state<string | null>(null);
	let busy = $state(false);

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
		estimates.ensureSubscribed();
		void estimates.load();
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
		formValidUntil = '';
		formNotes = '';
		lines = [];
		validationDetails = [];
		formError = null;
	}

	function openCreate(): void {
		resetForm();
		const today = new Date().toISOString().slice(0, 10);
		formIssueDate = today;
		formValidUntil = today;
		lines = [
			{
				kind: 'support',
				customItemId: null,
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
		const items: EstimateLineItemInput[] = lines.map((row, i) => ({
			supportItemId: null,
			customItemId: row.kind === 'custom' ? row.customItemId : null,
			catalogVersionId: null,
			code: row.kind === 'support' ? row.code : '',
			description: row.description,
			serviceDate: row.serviceDate,
			unit: row.unit,
			quantity: Number(row.quantity),
			unitPrice: Number(row.unitPrice),
			gstFree: row.gstFree,
			sortOrder: i
		}));
		return {
			participantId: Number(formParticipantId),
			planManagerId: null,
			status: 'draft' as EstimateStatus,
			issueDate: formIssueDate,
			validUntil: formValidUntil,
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
				await estimates.crud.create(payload);
			} else {
				await estimates.crud.update(editId, payload);
			}
			resetForm();
			await estimates.load();
		} catch (err) {
			if (err instanceof ApiError && err.status === 422) {
				validationDetails = err.details;
				formError = 'Please fix the highlighted line items.';
			} else {
				formError = err instanceof Error ? err.message : 'Failed to save estimate.';
			}
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
			formParticipantId = full.participantId === null ? '' : String(full.participantId);
			formIssueDate = full.issueDate ? full.issueDate.slice(0, 10) : '';
			formValidUntil = full.validUntil ? full.validUntil.slice(0, 10) : '';
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
				<p class="text-sm text-gray-500">Quote NDIS work before invoicing.</p>
			</div>
			<div class="flex items-center gap-2">
				<a
					href="/api/export/estimates"
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
					New estimate
				</button>
			</div>
		</div>

		{#if showForm}
			<form class="mb-8 space-y-4 rounded border border-gray-200 bg-white p-4" onsubmit={submitForm}>
				<h2 class="text-base font-semibold">
					{editId === null ? 'New estimate' : 'Edit estimate'}
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
						<span class="mb-1 block text-sm font-medium">Valid until</span>
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
						<th class="px-3 py-2 font-medium">Participant</th>
						<th class="px-3 py-2 font-medium">Issued</th>
						<th class="px-3 py-2 font-medium text-right">Total</th>
						<th class="px-3 py-2 font-medium">Status</th>
						<th class="px-3 py-2 font-medium text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered as est (est.id)}
						<tr class="border-b border-gray-100 last:border-0">
							<td class="px-3 py-2 font-medium">{est.number}</td>
							<td class="px-3 py-2 text-gray-600">{est.participantName || '—'}</td>
							<td class="px-3 py-2 text-gray-600">
								{est.issueDate ? est.issueDate.slice(0, 10) : '—'}
							</td>
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
								<button
									type="button"
									onclick={() => convert(est.id)}
									disabled={busy}
									class="mr-2 text-gray-900 hover:underline disabled:opacity-50"
								>
									Convert
								</button>
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
							<td colspan="6" class="px-3 py-6 text-center text-gray-500">No estimates found.</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</section>
</div>
