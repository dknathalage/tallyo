<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { apiPost, ApiError } from '$lib/api/client';
	import { shifts } from '$lib/stores/shifts.svelte';
	import { invoices as invoiceStore } from '$lib/stores/invoices.svelte';
	import { participants } from '$lib/stores/participants.svelte';
	import { dowDate, statusLabel } from '$lib/shifts/format';
	import type { Invoice, InvoiceStatus } from '$lib/api/types';

	const invoiceId = $derived(Number(page.params.id));

	let invoice = $state<Invoice | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let busy = $state(false);

	async function loadInvoice(): Promise<void> {
		if (!Number.isInteger(invoiceId) || invoiceId <= 0) {
			error = 'Invalid invoice id.';
			loading = false;
			return;
		}
		loading = true;
		error = null;
		try {
			// crud.get returns the full invoice (with line items) and throws on null.
			invoice = await invoiceStore.crud.get(invoiceId);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load invoice.';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		shifts.ensureSubscribed();
		void shifts.load();
		participants.ensureSubscribed();
		void participants.load();
		void loadInvoice();
	});

	function money(n: number): string {
		return '$' + (Number.isFinite(n) ? n : 0).toFixed(2);
	}

	// Source shifts: those attached to this invoice.
	const sourceShifts = $derived(
		shifts.items.filter((s) => s.invoiceId === invoiceId).sort((a, b) =>
			a.serviceDate < b.serviceDate ? -1 : 1
		)
	);

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

	// Next lifecycle action, mirroring the prototype: draft → sent → paid.
	const nextAction = $derived.by<{ label: string; status: InvoiceStatus } | null>(() => {
		if (!invoice) return null;
		if (invoice.status === 'draft') return { label: 'Mark sent', status: 'sent' };
		if (invoice.status === 'sent' || invoice.status === 'overdue')
			return { label: 'Mark paid', status: 'paid' };
		return null;
	});

	async function advance(status: InvoiceStatus): Promise<void> {
		if (!invoice) return;
		busy = true;
		error = null;
		try {
			await apiPost(`/api/invoices/${invoice.id}/status`, { status });
			await loadInvoice();
			await shifts.load();
		} catch (err) {
			if (err instanceof ApiError) error = err.message;
			else error = err instanceof Error ? err.message : 'Failed to update status.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-6">
	<p class="text-sm text-gray-500">
		<a href="/invoices" class="text-blue-600 hover:underline">Invoices</a>
		<span> › {invoice?.number ?? 'Invoice'}</span>
	</p>

	{#if loading}
		<p class="text-sm text-gray-500">Loading…</p>
	{:else if error}
		<p class="text-sm text-red-600">{error}</p>
	{:else if invoice}
		<div class="flex flex-wrap items-start justify-between gap-4 rounded-lg border border-gray-200 bg-white p-4">
			<div>
				<div class="flex items-center gap-2">
					<b class="text-lg">{invoice.number}</b>
					<span
						class="inline-block rounded px-2 py-0.5 text-xs font-medium capitalize {statusBadgeClass(
							invoice.status
						)}"
					>
						{invoice.status}
					</span>
				</div>
				<p class="text-sm text-gray-500">
					{invoice.participantName || '—'} · issued {invoice.issueDate
						? invoice.issueDate.slice(0, 10)
						: '—'}
				</p>
			</div>
			<div class="text-right">
				<div class="text-xl font-bold">{money(invoice.total)}</div>
				<a
					href={`/api/invoices/${invoice.id}/pdf`}
					target="_blank"
					rel="noopener"
					class="text-sm text-blue-600 hover:underline"
				>
					PDF
				</a>
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
						{#each invoice.lineItems as li (li.id)}
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
							<td class="px-2 py-1.5 text-right tabular-nums">{money(invoice.total)}</td>
						</tr>
					</tbody>
				</table>
			</div>
		</section>

		<section class="rounded-lg border border-gray-200 bg-white p-4">
			<h2 class="mb-3 text-xs font-semibold tracking-wide text-gray-500 uppercase">
				Source shifts ({sourceShifts.length})
			</h2>
			{#each sourceShifts as s (s.id)}
				<a
					href={`/participants/${s.participantId}`}
					class="flex items-center justify-between gap-3 border-b border-gray-100 py-2 text-sm last:border-0"
				>
					<span>
						{dowDate(s.serviceDate)}
						{#if s.note}<span class="text-gray-500">· {s.note}</span>{/if}
					</span>
					<span class="text-gray-500">{statusLabel(s.status)}</span>
				</a>
			{:else}
				<p class="text-sm text-gray-500">No shifts are linked to this invoice.</p>
			{/each}
		</section>

		<div class="flex items-center justify-end gap-2">
			<a
				href="/invoices"
				class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
			>
				Back
			</a>
			{#if nextAction}
				<button
					type="button"
					disabled={busy}
					onclick={() => advance(nextAction.status)}
					class="rounded bg-green-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{busy ? 'Saving…' : nextAction.label}
				</button>
			{:else}
				<span
					class="inline-block rounded px-3 py-2 text-sm font-medium {statusBadgeClass(
						invoice.status
					)}"
				>
					Paid ✓
				</span>
			{/if}
		</div>
	{:else}
		<p class="text-sm text-gray-500">Invoice not found.</p>
	{/if}
</div>
