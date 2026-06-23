<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { apiPost, ApiError, tenantPath } from '$lib/api/client';
	import { t } from '$lib/nav';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import LineItemsEditor from '$lib/components/LineItemsEditor.svelte';
	import type { EditorLine } from '$lib/components/LineItemsEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { invoices as invoiceStore } from '$lib/stores/invoices.svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import { sessions } from '$lib/stores/sessions.svelte';
	import { dowDate, statusLabel } from '$lib/sessions/format';
	import type {
		Invoice,
		InvoiceInput,
		InvoiceStatus,
		LineItemInput,
		ValidationDetail
	} from '$lib/api/types';

	const idParam = $derived((page.params.uuid ?? 'new'));

	function money(n: number): string {
		return '$' + (Number.isFinite(n) ? n : 0).toFixed(2);
	}

	// ── Flat editable fields (issue/due date, notes). Everything else on an
	// invoice (client, status, line items, totals) is derived/relational and
	// lives in the bespoke `extras` sections below. ───────────────────────────────
	const columns: Column<Invoice>[] = [
		{ key: 'issueDate', label: 'Issue date', input: 'date' },
		{ key: 'dueDate', label: 'Due date', input: 'date' },
		{ key: 'notes', label: 'Notes', input: 'textarea' }
	];

	// Build a full InvoiceInput from the loaded draft: the editor only mutates the
	// flat fields, but the API update is whole-document, so client, status and
	// the existing line items pass through unchanged (server re-derives totals).
	function toInput(inv: Invoice): InvoiceInput {
		const items: LineItemInput[] = inv.lineItems.map((li, i) => ({
			itemId: li.itemId,
			customItemId: li.customItemId,
			priceListVersionId: li.priceListVersionId,
			code: li.code,
			description: li.description,
			serviceDate: li.serviceDate,
			unit: li.unit,
			startTime: li.startTime,
			endTime: li.endTime,
			quantity: li.quantity,
			unitPrice: li.unitPrice,
			taxable: li.taxable,
			sortOrder: li.sortOrder ?? i
		}));
		return {
			clientId: inv.clientId,
			payerId: inv.payerId,
			status: inv.status,
			issueDate: inv.issueDate,
			dueDate: inv.dueDate,
			notes: inv.notes,
			lineItems: items
		};
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'issueDate' && String(value ?? '').trim() === '') return 'Issue date is required.';
		if (key === 'dueDate' && String(value ?? '').trim() === '') return 'Due date is required.';
		return null;
	}

	onMount(() => {
		clients.ensureSubscribed();
		void clients.load();
		customItems.ensureSubscribed();
		void customItems.load();
		businessProfile.subscribe();
		void businessProfile.load();
		sessions.ensureSubscribed();
		void sessions.load();
	});

	// ────────────────────────── Create flow (id === 'new') ──────────────────────
	// Invoices carry line items at creation, which a flat editor cannot capture, so
	// the create route hosts the full inline form (client + dates + lines).
	let formClientId = $state('');
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

	function seedCreate(): void {
		const today = new Date().toISOString().slice(0, 10);
		formIssueDate = today;
		formDueDate = today;
		lines = [
			{
				kind: 'support',
				customItemId: null,
				priceListVersionId: null,
				code: '',
				description: '',
				serviceDate: today,
				unit: '',
				quantity: 1,
				unitPrice: 0,
				taxable: false,
				sortOrder: 0
			}
		];
	}

	// Seed the create form exactly once when the route is the create route.
	let seeded = $state(false);
	$effect(() => {
		if (idParam === 'new' && !seeded) {
			seeded = true;
			seedCreate();
		}
	});

	function buildCreatePayload(): InvoiceInput {
		const items: LineItemInput[] = lines.map((row, i) => ({
			itemId: null,
			customItemId: row.kind === 'custom' ? row.customItemId : null,
			priceListVersionId: row.kind === 'support' ? row.priceListVersionId : null,
			code: row.kind === 'support' ? row.code : '',
			description: row.description,
			serviceDate: row.serviceDate,
			unit: row.unit,
			startTime: '',
			endTime: '',
			quantity: Number(row.quantity),
			unitPrice: Number(row.unitPrice),
			taxable: row.taxable,
			sortOrder: i
		}));
		return {
			clientId: formClientId,
			payerId: null,
			status: 'draft' as InvoiceStatus,
			issueDate: formIssueDate,
			dueDate: formDueDate,
			notes: formNotes,
			lineItems: items
		};
	}

	async function submitCreate(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		validationDetails = [];
		if (formClientId === '') {
			formError = 'Please select a client.';
			return;
		}
		if (lines.length === 0) {
			formError = 'Add at least one line item.';
			return;
		}
		saving = true;
		try {
			const created = await invoiceStore.crud.create(buildCreatePayload());
			await goto(t(`/invoices/${created.id}`));
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

	// ─────────────── Status lifecycle (bespoke; dedicated endpoint) ──────────────
	// The extras sections keep their own copy of the invoice so the lifecycle
	// buttons (which POST to /status, not the generic update) reflect immediately,
	// independently of the editor's draft.
	let detail = $state<Invoice | null>(null);
	let detailBusy = $state(false);
	let detailError = $state<string | null>(null);

	async function loadDetail(): Promise<void> {
		if (idParam === 'new') return;
		try {
			detail = await invoiceStore.crud.get(idParam);
		} catch (err) {
			detailError = err instanceof Error ? err.message : 'Failed to load invoice.';
		}
	}

	$effect(() => {
		if (idParam !== 'new') void loadDetail();
	});

	function statusBadgeClass(status: string): string {
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

	const nextAction = $derived.by<{ label: string; status: InvoiceStatus } | null>(() => {
		if (!detail) return null;
		if (detail.status === 'draft') return { label: 'Mark sent', status: 'sent' };
		if (detail.status === 'sent' || detail.status === 'overdue')
			return { label: 'Mark paid', status: 'paid' };
		return null;
	});

	async function advance(status: InvoiceStatus): Promise<void> {
		if (!detail) return;
		detailBusy = true;
		detailError = null;
		try {
			await apiPost(tenantPath(`invoices/${detail.id}/status`), { status });
			await loadDetail();
			await sessions.load();
		} catch (err) {
			if (err instanceof ApiError) detailError = err.message;
			else detailError = err instanceof Error ? err.message : 'Failed to update status.';
		} finally {
			detailBusy = false;
		}
	}

	// Source sessions: those attached to this invoice.
	const sourceSessions = $derived(
		idParam === 'new'
			? []
			: sessions.items
					.filter((s) => s.invoiceId === idParam)
					.sort((a, b) => (a.serviceDate < b.serviceDate ? -1 : 1))
	);
</script>

{#key idParam}
	{#if idParam === 'new'}
		<div class="space-y-5">
			<a href={t('/invoices')} class="text-sm text-gray-500 hover:text-gray-900">← Back</a>
			<h1 class="text-xl font-semibold">New invoice</h1>

			<form class="space-y-4 rounded border border-gray-200 bg-white p-4" onsubmit={submitCreate}>
				<div class="grid grid-cols-2 gap-3">
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Client</span>
						<select
							bind:value={formClientId}
							required
							class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
						>
							<option value="">— select —</option>
							{#each clients.items as p (p.id)}
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
					<a
						href={t('/invoices')}
						class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
					>
						Cancel
					</a>
				</div>
			</form>
		</div>
	{:else}
		<EntityEditor
			title="Invoice"
			{columns}
			crud={invoiceStore.crud}
			id={idParam}
			{toInput}
			{validate}
			backHref={t('/invoices')}
			{extras}
		/>
	{/if}
{/key}

{#snippet extras(row: Invoice)}
	<div class="space-y-6">
		<div
			class="flex flex-wrap items-start justify-between gap-4 rounded-lg border border-gray-200 bg-white p-4"
		>
			<div>
				<div class="flex items-center gap-2">
					<b class="text-lg">{row.number}</b>
					<span
						class="inline-block rounded px-2 py-0.5 text-xs font-medium capitalize {statusBadgeClass(
							(detail ?? row).status
						)}"
					>
						{(detail ?? row).status}
					</span>
				</div>
				<p class="text-sm text-gray-500">
					{row.clientName || '—'} · issued {row.issueDate ? row.issueDate.slice(0, 10) : '—'}
				</p>
			</div>
			<div class="text-right">
				<div class="text-xl font-bold">{money((detail ?? row).total)}</div>
			</div>
		</div>

		<section class="rounded-lg border border-gray-200 bg-white p-4">
			<h2 class="mb-3 text-xs font-semibold tracking-wide text-gray-500 uppercase">Line items</h2>
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead class="border-b border-gray-200 text-left text-gray-500">
						<tr>
							<th class="px-2 py-1.5 font-medium">Date</th>
							<th class="px-2 py-1.5 font-medium">Description</th>
							<th class="px-2 py-1.5 font-medium">Code</th>
							<th class="px-2 py-1.5 text-right font-medium">Qty</th>
							<th class="px-2 py-1.5 text-right font-medium">Unit</th>
							<th class="px-2 py-1.5 text-right font-medium">Amount</th>
						</tr>
					</thead>
					<tbody>
						{#each (detail ?? row).lineItems as li (li.id)}
							<tr class="border-b border-gray-100 last:border-0">
								<td class="px-2 py-1.5 text-gray-600">
									{li.serviceDate ? li.serviceDate.slice(0, 10) : '—'}
								</td>
								<td class="px-2 py-1.5">{li.description}</td>
								<td class="px-2 py-1.5 font-mono text-xs text-gray-500">{li.code || '—'}</td>
								<td class="px-2 py-1.5 text-right tabular-nums">{li.quantity}</td>
								<td class="px-2 py-1.5 text-right tabular-nums">{money(li.unitPrice)}</td>
								<td class="px-2 py-1.5 text-right tabular-nums">{money(li.lineTotal)}</td>
							</tr>
						{/each}
						<tr class="border-t-2 border-gray-300 font-semibold">
							<td class="px-2 py-1.5" colspan="5">Total</td>
							<td class="px-2 py-1.5 text-right tabular-nums">{money((detail ?? row).total)}</td>
						</tr>
					</tbody>
				</table>
			</div>
		</section>

		<section class="rounded-lg border border-gray-200 bg-white p-4">
			<h2 class="mb-3 text-xs font-semibold tracking-wide text-gray-500 uppercase">
				Source sessions ({sourceSessions.length})
			</h2>
			{#each sourceSessions as s (s.id)}
				<a
					href={t(`/clients/${s.clientId}`)}
					class="flex items-center justify-between gap-3 border-b border-gray-100 py-2 text-sm last:border-0"
				>
					<span>
						{dowDate(s.serviceDate)}
						{#if s.note}<span class="text-gray-500">· {s.note}</span>{/if}
					</span>
					<span class="text-gray-500">{statusLabel(s.status)}</span>
				</a>
			{:else}
				<p class="text-sm text-gray-500">No sessions are linked to this invoice.</p>
			{/each}
		</section>

		<div class="flex items-center justify-end gap-2">
			{#if detailError}
				<span class="mr-auto text-sm text-red-600">{detailError}</span>
			{/if}
			{#if nextAction}
				<button
					type="button"
					disabled={detailBusy}
					onclick={() => nextAction && advance(nextAction.status)}
					class="rounded bg-green-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{detailBusy ? 'Saving…' : nextAction.label}
				</button>
			{:else}
				<span
					class="inline-block rounded px-3 py-2 text-sm font-medium {statusBadgeClass(
						(detail ?? row).status
					)}"
				>
					Paid ✓
				</span>
			{/if}
		</div>
	</div>
{/snippet}
