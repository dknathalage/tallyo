<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { apiPost, ApiError } from '$lib/api/client';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import LineItemsEditor from '$lib/components/LineItemsEditor.svelte';
	import type { EditorLine } from '$lib/components/LineItemsEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { estimates as estimateStore } from '$lib/stores/estimates.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { customItems } from '$lib/stores/customItems.svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import type {
		Estimate,
		EstimateInput,
		EstimateStatus,
		EstimateLineItemInput,
		ValidationDetail
	} from '$lib/api/types';

	const idParam = $derived(page.params.id === 'new' ? 'new' : Number(page.params.id));

	function money(n: number): string {
		return '$' + (Number.isFinite(n) ? n : 0).toFixed(2);
	}

	// ── Flat editable fields (issue / valid-until date, notes). Everything else on
	// an estimate (participant, status, line items, totals) is derived/relational
	// and lives in the bespoke `extras` sections below. ────────────────────────────
	const columns: Column<Estimate>[] = [
		{ key: 'issueDate', label: 'Issue date', input: 'date' },
		{ key: 'validUntil', label: 'Valid until', input: 'date' },
		{ key: 'notes', label: 'Notes', input: 'textarea' }
	];

	// Build a full EstimateInput from the loaded draft: the editor only mutates the
	// flat fields, but the API update is whole-document, so participant, status and
	// the existing line items pass through unchanged (server re-derives totals).
	function toInput(est: Estimate): EstimateInput {
		const items: EstimateLineItemInput[] = est.lineItems.map((li, i) => ({
			supportItemId: li.supportItemId,
			customItemId: li.customItemId,
			catalogVersionId: li.catalogVersionId,
			code: li.code,
			description: li.description,
			serviceDate: li.serviceDate,
			unit: li.unit,
			startTime: li.startTime,
			endTime: li.endTime,
			quantity: li.quantity,
			unitPrice: li.unitPrice,
			gstFree: li.gstFree,
			sortOrder: li.sortOrder ?? i
		}));
		return {
			participantId: est.participantId ?? 0,
			planManagerId: est.planManagerId,
			status: est.status,
			issueDate: est.issueDate,
			validUntil: est.validUntil,
			notes: est.notes,
			lineItems: items
		};
	}

	function validate(key: string, value: unknown): string | null {
		if (key === 'issueDate' && String(value ?? '').trim() === '') return 'Issue date is required.';
		if (key === 'validUntil' && String(value ?? '').trim() === '')
			return 'Valid-until date is required.';
		return null;
	}

	onMount(() => {
		participants.ensureSubscribed();
		void participants.load();
		customItems.ensureSubscribed();
		void customItems.load();
		businessProfile.subscribe();
		void businessProfile.load();
	});

	// ────────────────────────── Create flow (id === 'new') ──────────────────────
	// Estimates carry line items at creation, which a flat editor cannot capture, so
	// the create route hosts the full inline form (participant + dates + lines).
	let formParticipantId = $state('');
	let formIssueDate = $state('');
	let formValidUntil = $state('');
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
	}

	// Seed the create form exactly once when the route is the create route.
	let seeded = $state(false);
	$effect(() => {
		if (idParam === 'new' && !seeded) {
			seeded = true;
			seedCreate();
		}
	});

	function buildCreatePayload(): EstimateInput {
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

	async function submitCreate(e: SubmitEvent): Promise<void> {
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
			const created = await estimateStore.crud.create(buildCreatePayload());
			await goto(`/estimates/${created.id}`);
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

	// ─────────────── Status lifecycle + conversion (bespoke endpoints) ───────────
	// The extras sections keep their own copy of the estimate so the lifecycle
	// buttons (which POST to /status, /convert, /duplicate, not the generic update)
	// reflect immediately, independently of the editor's draft.
	let detail = $state<Estimate | null>(null);
	let detailBusy = $state(false);
	let detailError = $state<string | null>(null);

	async function loadDetail(): Promise<void> {
		if (idParam === 'new') return;
		try {
			detail = await estimateStore.crud.get(idParam);
		} catch (err) {
			detailError = err instanceof Error ? err.message : 'Failed to load estimate.';
		}
	}

	$effect(() => {
		if (idParam !== 'new') void loadDetail();
	});

	function statusBadgeClass(status: string): string {
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

	async function setStatus(status: EstimateStatus): Promise<void> {
		if (!detail) return;
		detailBusy = true;
		detailError = null;
		try {
			await apiPost(`/api/estimates/${detail.id}/status`, { status });
			await loadDetail();
		} catch (err) {
			detailError = err instanceof Error ? err.message : 'Failed to update status.';
		} finally {
			detailBusy = false;
		}
	}

	async function convert(): Promise<void> {
		if (!detail) return;
		detailBusy = true;
		detailError = null;
		try {
			await apiPost(`/api/estimates/${detail.id}/convert`, {});
			await loadDetail();
		} catch (err) {
			detailError = err instanceof Error ? err.message : 'Failed to convert estimate.';
		} finally {
			detailBusy = false;
		}
	}

	async function duplicate(): Promise<void> {
		if (!detail) return;
		detailBusy = true;
		detailError = null;
		try {
			const created = await apiPost<Estimate>(`/api/estimates/${detail.id}/duplicate`, {});
			if (created) await goto(`/estimates/${created.id}`);
			else await loadDetail();
		} catch (err) {
			detailError = err instanceof Error ? err.message : 'Failed to duplicate estimate.';
		} finally {
			detailBusy = false;
		}
	}
</script>

{#if idParam === 'new'}
	<div class="space-y-5">
		<a href="/estimates" class="text-sm text-gray-500 hover:text-gray-900">← Back</a>
		<h1 class="text-xl font-semibold">New estimate</h1>

		<form class="space-y-4 rounded border border-gray-200 bg-white p-4" onsubmit={submitCreate}>
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
					{saving ? 'Saving…' : 'Create estimate'}
				</button>
				<a
					href="/estimates"
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
				>
					Cancel
				</a>
			</div>
		</form>
	</div>
{:else}
	<EntityEditor
		title="Estimate"
		{columns}
		crud={estimateStore.crud}
		id={idParam}
		{toInput}
		{validate}
		backHref="/estimates"
		{extras}
	/>
{/if}

{#snippet extras(row: Estimate)}
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
					{row.participantName || '—'} · issued {row.issueDate ? row.issueDate.slice(0, 10) : '—'}
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

		{#if (detail ?? row).convertedInvoiceId}
			<section class="rounded-lg border border-gray-200 bg-white p-4">
				<h2 class="mb-2 text-xs font-semibold tracking-wide text-gray-500 uppercase">Converted</h2>
				<a
					href={`/invoices/${(detail ?? row).convertedInvoiceId}`}
					class="text-sm text-blue-700 underline"
				>
					View resulting invoice →
				</a>
			</section>
		{/if}

		<div class="flex flex-wrap items-center justify-end gap-2">
			{#if detailError}
				<span class="mr-auto text-sm text-red-600">{detailError}</span>
			{/if}
			{#if (detail ?? row).status === 'draft'}
				<button
					type="button"
					disabled={detailBusy}
					onclick={() => setStatus('accepted')}
					class="rounded bg-green-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{detailBusy ? 'Saving…' : 'Accept'}
				</button>
				<button
					type="button"
					disabled={detailBusy}
					onclick={() => setStatus('declined')}
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50 disabled:opacity-50"
				>
					Decline
				</button>
			{/if}
			{#if (detail ?? row).status !== 'converted'}
				<button
					type="button"
					disabled={detailBusy}
					onclick={convert}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					Convert to invoice
				</button>
			{/if}
			<button
				type="button"
				disabled={detailBusy}
				onclick={duplicate}
				class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50 disabled:opacity-50"
			>
				Duplicate
			</button>
		</div>
	</div>
{/snippet}
