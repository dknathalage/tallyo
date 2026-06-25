<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { apiPost, ApiError, tenantPath } from '$lib/api/client';
	import { t } from '$lib/nav';
	import EntityEditor from '$lib/components/EntityEditor.svelte';
	import LineItemsEditor from '$lib/components/LineItemsEditor.svelte';
	import Button from '$lib/components/Button.svelte';
	import Badge from '$lib/components/Badge.svelte';
	import type { EditorLine } from '$lib/components/LineItemsEditor.svelte';
	import type { Column } from '$lib/components/datatable';
	import { estimates as estimateStore } from '$lib/stores/estimates.svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { catalogue } from '$lib/stores/catalogue.svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import type {
		Estimate,
		EstimateInput,
		EstimateStatus,
		EstimateLineItemInput,
		ValidationDetail
	} from '$lib/api/types';

	const idParam = $derived((page.params.uuid ?? 'new'));

	function money(n: number): string {
		return '$' + (Number.isFinite(n) ? n : 0).toFixed(2);
	}

	// ── Flat editable fields (issue / valid-until date, notes). Everything else on
	// an estimate (client, status, line items, totals) is derived/relational
	// and lives in the bespoke `extras` sections below. ────────────────────────────
	const columns: Column<Estimate>[] = [
		{ key: 'issueDate', label: 'Issue date', input: 'date' },
		{ key: 'validUntil', label: 'Valid until', input: 'date' },
		{ key: 'notes', label: 'Notes', input: 'textarea' }
	];

	// Build a full EstimateInput from the loaded draft: the editor only mutates the
	// flat fields, but the API update is whole-document, so client, status and
	// the existing line items pass through unchanged (server re-derives totals).
	function toInput(est: Estimate): EstimateInput {
		const items: EstimateLineItemInput[] = est.lineItems.map((li, i) => ({
			catalogueItemId: li.catalogueItemId,
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
			clientId: est.clientId ?? '',
			payerId: est.payerId,
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
		clients.ensureSubscribed();
		void clients.load();
		catalogue.ensureSubscribed();
		void catalogue.load();
		businessProfile.subscribe();
		void businessProfile.load();
	});

	// ────────────────────────── Create flow (id === 'new') ──────────────────────
	// Estimates carry line items at creation, which a flat editor cannot capture, so
	// the create route hosts the full inline form (client + dates + lines).
	let formClientId = $state('');
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
				kind: 'catalogue',
				catalogueItemId: null,
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

	function buildCreatePayload(): EstimateInput {
		const items: EstimateLineItemInput[] = lines.map((row, i) => ({
			catalogueItemId: row.kind === 'catalogue' ? row.catalogueItemId : null,
			code: row.kind === 'catalogue' ? row.code : '',
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
			const created = await estimateStore.crud.create(buildCreatePayload());
			await goto(t(`/estimates/${created.id}`));
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

	function statusTone(status: string): 'green' | 'blue' | 'red' | 'slate' {
		switch (status) {
			case 'accepted':
				return 'green';
			case 'converted':
				return 'blue';
			case 'declined':
				return 'red';
			default:
				return 'slate';
		}
	}

	async function setStatus(status: EstimateStatus): Promise<void> {
		if (!detail) return;
		detailBusy = true;
		detailError = null;
		try {
			await apiPost(tenantPath(`estimates/${detail.id}/status`), { status });
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
			await apiPost(tenantPath(`estimates/${detail.id}/convert`), {});
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
			const created = await apiPost<Estimate>(tenantPath(`estimates/${detail.id}/duplicate`), {});
			if (created) await goto(t(`/estimates/${created.id}`));
			else await loadDetail();
		} catch (err) {
			detailError = err instanceof Error ? err.message : 'Failed to duplicate estimate.';
		} finally {
			detailBusy = false;
		}
	}
</script>

{#key idParam}
	{#if idParam === 'new'}
		<div class="space-y-5">
			<a href={t('/estimates')} class="text-sm text-gray-500 hover:text-gray-900">← Back</a>
			<h1 class="text-2xl font-semibold tracking-tight">New estimate</h1>

			<form
				class="space-y-4 rounded-xl border border-gray-200 bg-white p-4 shadow-sm"
				onsubmit={submitCreate}
			>
				<div class="grid grid-cols-2 gap-3">
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Client</span>
						<select
							bind:value={formClientId}
							required
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
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
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
						/>
					</label>
					<label class="col-span-1">
						<span class="mb-1 block text-sm font-medium">Valid until</span>
						<input
							type="date"
							bind:value={formValidUntil}
							required
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
						/>
					</label>
					<label class="col-span-2">
						<span class="mb-1 block text-sm font-medium">Notes</span>
						<input
							type="text"
							bind:value={formNotes}
							class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
						/>
					</label>
				</div>

				<LineItemsEditor bind:lines details={validationDetails} />

				<div class="flex justify-end">
					<dl class="w-56 space-y-1 text-sm">
						<div class="flex justify-between">
							<dt class="text-gray-500">Subtotal (preview)</dt>
							<dd class="font-mono tabular-nums">{money(subtotalPreview)}</dd>
						</div>
						<p class="text-xs text-gray-400">Tax + total are calculated on save.</p>
					</dl>
				</div>

				{#if formError}
					<p class="text-sm text-red-600">{formError}</p>
				{/if}

				<div class="flex gap-2">
					<Button type="submit" disabled={saving} loading={saving}>
						{saving ? 'Saving…' : 'Create estimate'}
					</Button>
					<Button variant="secondary" href={t('/estimates')}>Cancel</Button>
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
			backHref={t('/estimates')}
			{extras}
		/>
	{/if}
{/key}

{#snippet extras(row: Estimate)}
	<div class="space-y-6">
		<div
			class="flex flex-wrap items-start justify-between gap-4 rounded-lg border border-gray-200 bg-white p-4"
		>
			<div>
				<div class="flex items-center gap-2">
					<b class="font-mono text-lg tabular-nums">{row.number}</b>
					<Badge tone={statusTone((detail ?? row).status)} class="capitalize">
						{(detail ?? row).status}
					</Badge>
				</div>
				<p class="text-sm text-gray-500">
					{row.clientName || '—'} · issued {row.issueDate ? row.issueDate.slice(0, 10) : '—'}
				</p>
			</div>
			<div class="text-right">
				<div class="font-mono text-xl font-bold tabular-nums">{money((detail ?? row).total)}</div>
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
								<td class="px-2 py-1.5 text-right font-mono tabular-nums">{li.quantity}</td>
								<td class="px-2 py-1.5 text-right font-mono tabular-nums">{money(li.unitPrice)}</td>
								<td class="px-2 py-1.5 text-right font-mono tabular-nums">{money(li.lineTotal)}</td>
							</tr>
						{/each}
						<tr class="border-t-2 border-gray-300 font-semibold">
							<td class="px-2 py-1.5" colspan="5">Total</td>
							<td class="px-2 py-1.5 text-right font-mono tabular-nums">{money((detail ?? row).total)}</td>
						</tr>
					</tbody>
				</table>
			</div>
		</section>

		{#if (detail ?? row).convertedInvoiceId}
			<section class="rounded-lg border border-gray-200 bg-white p-4">
				<h2 class="mb-2 text-xs font-semibold tracking-wide text-gray-500 uppercase">Converted</h2>
				<a
					href={t(`/invoices/${(detail ?? row).convertedInvoiceId}`)}
					class="text-sm text-brand-700 underline"
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
				<Button type="button" disabled={detailBusy} loading={detailBusy} onclick={() => setStatus('accepted')}>
					{detailBusy ? 'Saving…' : 'Accept'}
				</Button>
				<Button
					variant="secondary"
					type="button"
					disabled={detailBusy}
					onclick={() => setStatus('declined')}
				>
					Decline
				</Button>
			{/if}
			{#if (detail ?? row).status !== 'converted'}
				<Button variant="secondary" type="button" disabled={detailBusy} onclick={convert}
					>Convert to invoice</Button
				>
			{/if}
			<Button variant="secondary" type="button" disabled={detailBusy} onclick={duplicate}>
				Duplicate
			</Button>
		</div>
	</div>
{/snippet}
