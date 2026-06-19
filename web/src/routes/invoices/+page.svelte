<script lang="ts">
	import { onMount } from 'svelte';
	import { invoices } from '$lib/stores/invoices.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import { ApiError } from '$lib/api/client';
	import DataTable from '$lib/components/DataTable.svelte';
	import type { Column } from '$lib/components/datatable';
	import LineItemsEditor from '$lib/components/LineItemsEditor.svelte';
	import type { EditorLine } from '$lib/components/LineItemsEditor.svelte';
	import type { Invoice, InvoiceStatus, LineItemInput, ValidationDetail } from '$lib/api/types';

	const STATUSES: InvoiceStatus[] = ['draft', 'sent', 'overdue', 'paid'];

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	// Read-only payment-status label for the list. A drawer/detail page owns the
	// actual payment workflow; here we only summarise paid vs total.
	function paymentLabel(total: number, paid: number): string {
		const t = Number.isFinite(total) ? total : 0;
		const p = Number.isFinite(paid) ? paid : 0;
		if (p >= t && t > 0) return 'Paid';
		if (p > 0) return 'Partial';
		return 'Unpaid';
	}

	// ── Create form (the only inline edit path; documents are edited on detail) ──
	let showForm = $state(false);
	let formParticipantId = $state('');
	let formIssueDate = $state('');
	let formDueDate = $state('');
	let formNotes = $state('');
	let lines = $state<EditorLine[]>([]);
	let validationDetails = $state<ValidationDetail[]>([]);
	let saving = $state(false);
	let formError = $state<string | null>(null);

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
		void invoices.query({ page: 1, limit: 50 });
		participants.ensureSubscribed();
		void participants.load();
		customItems.ensureSubscribed();
		void customItems.load();
		businessProfile.subscribe();
		void businessProfile.load();
	});

	function resetForm(): void {
		showForm = false;
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
			await invoices.crud.create(buildPayload());
			resetForm();
			await invoices.query({ page: 1, limit: 50 });
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

	// DataTable column definitions. Keys match Invoice JSON fields (and the server
	// allowlist), so one key drives filter, sort and display. No onRowSave is
	// passed, so the drawer is read-only — invoices are edited on their detail page.
	const columns: Column<Invoice>[] = [
		{ key: 'number', label: 'Number', sortable: true, filter: 'text' },
		{ key: 'participantName', label: 'Participant', sortable: true, filter: 'text' },
		{
			key: 'issueDate',
			label: 'Issued',
			sortable: true,
			filter: 'date',
			cell: (inv) => (inv.issueDate ? inv.issueDate.slice(0, 10) : '—')
		},
		{
			key: 'total',
			label: 'Total',
			sortable: true,
			filter: 'number',
			cell: (inv) => money(inv.total)
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			filter: 'enum',
			values: STATUSES
		},
		{
			key: 'payment',
			label: 'Payment',
			cell: (inv) => paymentLabel(inv.total, inv.totalPaid)
		}
	];
</script>

<div class="space-y-6">
	<section>
		<div class="mb-2">
			<h1 class="mb-1 text-xl font-semibold">Invoices</h1>
			<p class="text-sm text-gray-500">NDIS-compliant invoices with price-cap validation.</p>
		</div>

		{#if showForm}
			<form class="mb-6 space-y-4 rounded border border-gray-200 bg-white p-4" onsubmit={submitForm}>
				<h2 class="text-base font-semibold">New invoice</h2>

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
						{saving ? 'Saving…' : 'Create invoice'}
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
		<DataTable
			title="Invoices"
			{columns}
			store={invoices}
			onNew={openCreate}
			detailHref={(inv) => `/invoices/${inv.id}`}
		/>
	</section>
</div>
