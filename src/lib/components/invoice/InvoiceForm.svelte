<script lang="ts">
	import { getClients, getClient } from '$lib/db/queries/clients.js';
	import { buildBusinessSnapshot } from '$lib/db/queries/business-profile.js';
	import { buildClientSnapshot } from '$lib/db/queries/clients.js';
	import { buildPayerSnapshot } from '$lib/db/queries/payers.js';
	import { getPayer } from '$lib/db/queries/payers.js';
	import { getRateTiers } from '$lib/db/queries/rate-tiers.js';
	import { generateInvoiceNumber } from '$lib/utils/invoice-number.js';
	import { today } from '$lib/utils/format.js';
	import type { Client, Invoice, LineItem, KeyValuePair, PartySnapshot } from '$lib/types/index.js';
	import Button from '$lib/components/shared/Button.svelte';
	import KeyValueEditor from '$lib/components/shared/KeyValueEditor.svelte';
	import LineItemRow from './LineItemRow.svelte';

	let {
		initialData,
		initialLineItems,
		onsubmit
	}: {
		initialData?: Invoice;
		initialLineItems?: LineItem[];
		onsubmit: (
			data: {
				invoice_number: string;
				client_id: number;
				date: string;
				due_date: string;
				subtotal: number;
				tax_rate: number;
				tax_amount: number;
				total: number;
				notes: string;
				status: string;
				business_snapshot: string;
				client_snapshot: string;
				payer_snapshot: string;
			},
			lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number; notes: string }>
		) => void;
	} = $props();

	let clients: Client[] = $state([]);
	let invoiceNumber = $state(initialData?.invoice_number ?? '');
	let clientId = $state(initialData?.client_id ?? 0);
	let date = $state(initialData?.date ?? today());
	let dueDate = $state(initialData?.due_date ?? today());
	let taxRate = $state(initialData?.tax_rate ?? 0);
	let notes = $state(initialData?.notes ?? '');
	let status = $state(initialData?.status ?? 'draft');

	let lineItems = $state<Array<{ description: string; quantity: number; rate: number; amount: number; unit?: string; notes?: string }>>(
		initialLineItems?.map((li) => ({
			description: li.description,
			quantity: li.quantity,
			rate: li.rate,
			amount: li.amount,
			notes: li.notes ?? ''
		})) ?? [{ description: '', quantity: 1, rate: 0, amount: 0, unit: undefined, notes: '' }]
	);

	let selectedClient = $derived(clientId ? getClient(clientId) : null);
	let activeTierId = $derived(selectedClient?.pricing_tier_id ?? null);
	let tiers = $derived(getRateTiers());
	let activeTierName = $derived(tiers.find(t => t.id === activeTierId)?.name ?? null);

	let subtotal = $derived(
		Math.round(lineItems.reduce((sum, item) => sum + item.amount, 0) * 100) / 100
	);
	let taxAmount = $derived(Math.round(subtotal * (taxRate / 100) * 100) / 100);
	let total = $derived(Math.round((subtotal + taxAmount) * 100) / 100);

	// --- Snapshot helpers ---
	function parseMetadata(metaStr?: string): KeyValuePair[] {
		try {
			const obj = JSON.parse(metaStr || '{}');
			return Object.entries(obj).map(([key, value]) => ({ key, value: String(value) }));
		} catch {
			return [];
		}
	}

	function pairsToRecord(pairs: KeyValuePair[]): Record<string, string> {
		const obj: Record<string, string> = {};
		for (const pair of pairs) {
			if (pair.key.trim()) obj[pair.key.trim()] = pair.value;
		}
		return obj;
	}

	function parseSnapshot(json: string): PartySnapshot {
		try {
			const p = JSON.parse(json || '{}');
			return { name: p.name || '', email: p.email || '', phone: p.phone || '', address: p.address || '', logo: p.logo, metadata: p.metadata || {} };
		} catch {
			return { name: '', email: '', phone: '', address: '', metadata: {} };
		}
	}

	// Business snapshot (read-only, always from current profile)
	let businessSnapshot: PartySnapshot = $state({ name: '', email: '', phone: '', address: '', metadata: {} });

	// Client metadata (editable on invoice)
	let clientMetadataPairs: KeyValuePair[] = $state([]);

	// Payer info (editable on invoice)
	let payerName = $state('');
	let payerEmail = $state('');
	let payerPhone = $state('');
	let payerAddress = $state('');
	let payerMetadataPairs: KeyValuePair[] = $state([]);
	let hasPayer = $derived(payerName.trim().length > 0);

	// Initialize clients, invoice number, business snapshot, and edit-mode snapshots
	$effect(() => {
		clients = getClients();
		if (!initialData) {
			invoiceNumber = generateInvoiceNumber();
		}

		// Load business snapshot
		businessSnapshot = buildBusinessSnapshot();

		// If editing existing invoice, load snapshots from the invoice
		if (initialData) {
			const cs = parseSnapshot(initialData.client_snapshot);
			clientMetadataPairs = Object.entries(cs.metadata).map(([key, value]) => ({ key, value }));

			const ps = parseSnapshot(initialData.payer_snapshot);
			payerName = ps.name;
			payerEmail = ps.email;
			payerPhone = ps.phone;
			payerAddress = ps.address;
			payerMetadataPairs = Object.entries(ps.metadata).map(([key, value]) => ({ key, value }));
		}
	});

	// Auto-populate client metadata and payer when client changes (new invoices only)
	$effect(() => {
		if (!clientId || initialData) return;

		// Auto-populate client metadata from client record
		const client = getClient(clientId);
		if (client) {
			clientMetadataPairs = parseMetadata(client.metadata);

			// Auto-populate payer from client's linked payer
			if (client.payer_id) {
				const payer = getPayer(client.payer_id);
				if (payer) {
					payerName = payer.name;
					payerEmail = payer.email;
					payerPhone = payer.phone;
					payerAddress = payer.address;
					payerMetadataPairs = parseMetadata(payer.metadata);
				} else {
					payerName = '';
					payerEmail = '';
					payerPhone = '';
					payerAddress = '';
					payerMetadataPairs = [];
				}
			} else {
				payerName = '';
				payerEmail = '';
				payerPhone = '';
				payerAddress = '';
				payerMetadataPairs = [];
			}
		}
	});

	function addLineItem() {
		lineItems.push({ description: '', quantity: 1, rate: 0, amount: 0, unit: undefined, notes: '' });
	}

	function removeLineItem(index: number) {
		lineItems.splice(index, 1);
	}

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();

		// Build client snapshot
		const clientSnapshotObj: PartySnapshot = {
			name: selectedClient?.name ?? '',
			email: selectedClient?.email ?? '',
			phone: selectedClient?.phone ?? '',
			address: selectedClient?.address ?? '',
			metadata: pairsToRecord(clientMetadataPairs)
		};

		// Build payer snapshot
		const payerSnapshotObj: PartySnapshot = {
			name: payerName,
			email: payerEmail,
			phone: payerPhone,
			address: payerAddress,
			metadata: pairsToRecord(payerMetadataPairs)
		};

		onsubmit(
			{
				invoice_number: invoiceNumber,
				client_id: clientId,
				date,
				due_date: dueDate,
				subtotal,
				tax_rate: taxRate,
				tax_amount: taxAmount,
				total,
				notes,
				status,
				business_snapshot: JSON.stringify(businessSnapshot),
				client_snapshot: JSON.stringify(clientSnapshotObj),
				payer_snapshot: JSON.stringify(payerSnapshotObj)
			},
			lineItems.map((item, i) => ({
				description: item.description,
				quantity: item.quantity,
				rate: item.rate,
				amount: item.amount,
				sort_order: i,
				notes: item.notes ?? ''
			}))
		);
	}
</script>

<form onsubmit={handleSubmit} class="space-y-6">
	<!-- Business Profile (read-only) -->
	{#if businessSnapshot.name}
		<div class="rounded-lg border border-gray-200 bg-gray-50 p-4">
			<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500">From</h3>
			<p class="mt-1 text-sm font-medium text-gray-900">{businessSnapshot.name}</p>
			{#if businessSnapshot.email}<p class="text-sm text-gray-500">{businessSnapshot.email}</p>{/if}
			{#if businessSnapshot.phone}<p class="text-sm text-gray-500">{businessSnapshot.phone}</p>{/if}
			{#if businessSnapshot.address}<p class="whitespace-pre-line text-sm text-gray-500">{businessSnapshot.address}</p>{/if}
			{#if Object.keys(businessSnapshot.metadata).length > 0}
				<div class="mt-1 space-y-0.5">
					{#each Object.entries(businessSnapshot.metadata) as [key, value]}
						<p class="text-sm text-gray-500"><span class="font-medium text-gray-700">{key}:</span> {value}</p>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	<!-- Header fields -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
		<div>
			<label for="invoice-number" class="block text-sm font-medium text-gray-700">Invoice Number</label>
			<input
				id="invoice-number"
				type="text"
				bind:value={invoiceNumber}
				readonly
				class="mt-1 w-full rounded-lg border border-gray-300 bg-gray-50 px-3 py-2 text-sm text-gray-900 focus:outline-none"
			/>
		</div>

		<div>
			<label for="client" class="block text-sm font-medium text-gray-700">Client</label>
			<select
				id="client"
				bind:value={clientId}
				required
				class="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			>
				<option value={0} disabled>Select a client</option>
				{#each clients as client}
					<option value={client.id}>{client.name}</option>
				{/each}
			</select>
			{#if activeTierName}
				<p class="mt-1 text-xs text-gray-500">Pricing: {activeTierName}</p>
			{/if}
		</div>

		<div>
			<label for="date" class="block text-sm font-medium text-gray-700">Date</label>
			<input
				id="date"
				type="date"
				bind:value={date}
				required
				class="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			/>
		</div>

		<div>
			<label for="due-date" class="block text-sm font-medium text-gray-700">Due Date</label>
			<input
				id="due-date"
				type="date"
				bind:value={dueDate}
				required
				class="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			/>
		</div>
	</div>

	<!-- Line Items -->
	<div>
		<h3 class="mb-3 text-sm font-medium text-gray-700">Line Items</h3>

		<!-- Header -->
		<div class="mb-2 flex items-center gap-3 text-xs font-medium uppercase tracking-wide text-gray-500">
			<div class="flex-1">Description</div>
			<div class="w-24">Qty</div>
			<div class="w-28">Rate</div>
			<div class="w-28 text-right">Amount</div>
			<div class="w-8"></div>
		</div>

		<div class="space-y-2">
			{#each lineItems as _, i}
				<LineItemRow bind:item={lineItems[i]} onremove={() => removeLineItem(i)} tierId={activeTierId} />
			{/each}
		</div>

		<button
			type="button"
			onclick={addLineItem}
			class="mt-3 cursor-pointer text-sm font-medium text-primary-600 hover:text-primary-700"
		>
			+ Add Line Item
		</button>
	</div>

	<!-- Client Metadata -->
	{#if clientId}
		<div>
			<h3 class="mb-2 text-sm font-medium text-gray-700">Client Additional Fields</h3>
			<KeyValueEditor bind:pairs={clientMetadataPairs} addLabel="Add Field" />
		</div>
	{/if}

	<!-- Payer / Bill-To -->
	{#if clientId}
		<div class="rounded-lg border border-gray-200 p-4">
			<h3 class="mb-3 text-sm font-medium text-gray-700">Bill To (Payer)</h3>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<div>
					<label for="payer-name" class="block text-xs font-medium text-gray-500">Name</label>
					<input
						id="payer-name"
						type="text"
						bind:value={payerName}
						placeholder="Payer name (optional)"
						class="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
					/>
				</div>
				<div>
					<label for="payer-email" class="block text-xs font-medium text-gray-500">Email</label>
					<input
						id="payer-email"
						type="email"
						bind:value={payerEmail}
						placeholder="Payer email"
						class="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
					/>
				</div>
				<div>
					<label for="payer-phone" class="block text-xs font-medium text-gray-500">Phone</label>
					<input
						id="payer-phone"
						type="tel"
						bind:value={payerPhone}
						placeholder="Payer phone"
						class="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
					/>
				</div>
				<div>
					<label for="payer-address" class="block text-xs font-medium text-gray-500">Address</label>
					<input
						id="payer-address"
						type="text"
						bind:value={payerAddress}
						placeholder="Payer address"
						class="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
					/>
				</div>
			</div>
			<div class="mt-3">
				<label class="block text-xs font-medium text-gray-500">Additional Fields</label>
				<div class="mt-1">
					<KeyValueEditor bind:pairs={payerMetadataPairs} addLabel="Add Field" />
				</div>
			</div>
		</div>
	{/if}

	<!-- Tax and totals -->
	<div class="flex justify-end">
		<div class="w-72 space-y-2">
			<div class="flex justify-between text-sm">
				<span class="text-gray-600">Subtotal</span>
				<span class="font-medium text-gray-900">${subtotal.toFixed(2)}</span>
			</div>

			<div class="flex items-center justify-between gap-3 text-sm">
				<label for="tax-rate" class="text-gray-600">Tax Rate (%)</label>
				<input
					id="tax-rate"
					type="number"
					bind:value={taxRate}
					min="0"
					step="any"
					class="w-20 rounded-lg border border-gray-300 px-2 py-1 text-right text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				/>
			</div>

			<div class="flex justify-between text-sm">
				<span class="text-gray-600">Tax</span>
				<span class="font-medium text-gray-900">${taxAmount.toFixed(2)}</span>
			</div>

			<div class="flex justify-between border-t border-gray-200 pt-2 text-base">
				<span class="font-semibold text-gray-900">Total</span>
				<span class="font-semibold text-gray-900">${total.toFixed(2)}</span>
			</div>
		</div>
	</div>

	<!-- Notes -->
	<div>
		<label for="notes" class="block text-sm font-medium text-gray-700">Notes</label>
		<textarea
			id="notes"
			bind:value={notes}
			rows={3}
			placeholder="Additional notes..."
			class="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		></textarea>
	</div>

	<!-- Actions -->
	<div class="flex justify-end gap-3">
		<Button type="submit">
			{initialData ? 'Update Invoice' : 'Create Invoice'}
		</Button>
	</div>
</form>
