<script lang="ts">
	import { onMount } from 'svelte';
	import { estimates } from '$lib/stores/estimates.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import { ApiError, apiPost } from '$lib/api/client';
	import LineItemsEditor from '$lib/components/LineItemsEditor.svelte';
	import type { EditorLine } from '$lib/components/LineItemsEditor.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column, RowAction } from '$lib/components/datatable';
	import Pencil from '@lucide/svelte/icons/pencil';
	import FileOutput from '@lucide/svelte/icons/file-output';
	import Copy from '@lucide/svelte/icons/copy';
	import Check from '@lucide/svelte/icons/check';
	import Ban from '@lucide/svelte/icons/ban';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import type {
		Estimate,
		EstimateStatus,
		EstimateLineItemInput,
		ValidationDetail
	} from '$lib/api/types';

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	// DataTable column definitions. Keys match Estimate JSON fields (and the
	// server allowlist), so one key drives filter, sort and display.
	const columns: Column<Estimate>[] = [
		{ key: 'number', label: 'Number', sortable: true, filter: 'text' },
		{ key: 'participantName', label: 'Participant', sortable: true, filter: 'text' },
		{
			key: 'issueDate',
			label: 'Issued',
			sortable: true,
			filter: 'date',
			cell: (e) => (e.issueDate ? e.issueDate.slice(0, 10) : '—')
		},
		{
			key: 'total',
			label: 'Total',
			sortable: true,
			filter: 'number',
			cell: (e) => money(e.total)
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			filter: 'enum',
			values: ['draft', 'accepted', 'declined', 'converted']
		}
	];

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

	// Load a full estimate (with line items) into the shared form for editing.
	async function startEdit(id: number): Promise<void> {
		rowError = null;
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
			formError = null;
			showForm = true;
		} catch (err) {
			rowError = err instanceof Error ? err.message : 'Failed to load estimate.';
		}
	}

	// Table actions. Edit operates on the first selected row; the rest loop over
	// the selection. Mutations refresh via the SSE estimate event re-running the
	// active query.
	const rowActions: RowAction<Estimate>[] = [
		{
			label: 'Edit',
			icon: Pencil,
			bulk: true,
			run: async (rows) => {
				if (rows.length > 0) await startEdit(rows[0].id);
			}
		},
		{
			label: 'Convert to invoice',
			icon: FileOutput,
			bulk: true,
			run: async (rows) => {
				rowError = null;
				try {
					for (const r of rows) await apiPost(`/api/estimates/${r.id}/convert`, {});
				} catch (err) {
					rowError = err instanceof Error ? err.message : 'Failed to convert estimate.';
				}
			}
		},
		{
			label: 'Duplicate',
			icon: Copy,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await apiPost(`/api/estimates/${r.id}/duplicate`, {});
			}
		},
		{
			label: 'Accept',
			icon: Check,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await apiPost(`/api/estimates/${r.id}/status`, { status: 'accepted' });
			}
		},
		{
			label: 'Decline',
			icon: Ban,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await apiPost(`/api/estimates/${r.id}/status`, { status: 'declined' });
			}
		},
		{
			label: 'Delete',
			icon: Trash2,
			danger: true,
			bulk: true,
			run: async (rows) => {
				for (const r of rows) await estimates.crud.remove(r.id);
			}
		}
	];

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
		void estimates.query({ page: 1, limit: 50 });
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
		const items: EstimateLineItemInput[] = lines.map((row, i) => ({
			supportItemId: null,
			customItemId: row.kind === 'custom' ? row.customItemId : null,
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

</script>

<div class="space-y-8">
	<section>
		<div class="mb-6 flex items-center justify-between">
			<div>
				<h1 class="mb-1 text-xl font-semibold">Estimates</h1>
				<p class="text-sm text-gray-500">Quote NDIS work before invoicing.</p>
			</div>
			<div class="flex items-center gap-2">
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
		{#if estimates.error}
			<p class="mb-3 text-sm text-red-600">{estimates.error}</p>
		{/if}
		{#if rowError}
			<p class="mb-3 text-sm text-red-600">{rowError}</p>
		{/if}

		<DataTable title="Estimates" {columns} store={estimates} {rowActions} onNew={openCreate} />
	</section>
</div>
