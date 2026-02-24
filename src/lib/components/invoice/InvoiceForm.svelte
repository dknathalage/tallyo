<script lang="ts">
	import { getClients } from '$lib/db/queries/clients.js';
	import { generateInvoiceNumber } from '$lib/utils/invoice-number.js';
	import { today } from '$lib/utils/format.js';
	import type { Client, Invoice, LineItem } from '$lib/types/index.js';
	import Button from '$lib/components/shared/Button.svelte';
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
			},
			lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; sort_order: number }>
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

	let lineItems = $state<Array<{ description: string; quantity: number; rate: number; amount: number }>>(
		initialLineItems?.map((li) => ({
			description: li.description,
			quantity: li.quantity,
			rate: li.rate,
			amount: li.amount
		})) ?? [{ description: '', quantity: 1, rate: 0, amount: 0 }]
	);

	let subtotal = $derived(
		Math.round(lineItems.reduce((sum, item) => sum + item.amount, 0) * 100) / 100
	);
	let taxAmount = $derived(Math.round(subtotal * (taxRate / 100) * 100) / 100);
	let total = $derived(Math.round((subtotal + taxAmount) * 100) / 100);

	$effect(() => {
		clients = getClients();
		if (!initialData) {
			invoiceNumber = generateInvoiceNumber();
		}
	});

	function addLineItem() {
		lineItems.push({ description: '', quantity: 1, rate: 0, amount: 0 });
	}

	function removeLineItem(index: number) {
		lineItems.splice(index, 1);
	}

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
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
				status
			},
			lineItems.map((item, i) => ({
				description: item.description,
				quantity: item.quantity,
				rate: item.rate,
				amount: item.amount,
				sort_order: i
			}))
		);
	}
</script>

<form onsubmit={handleSubmit} class="space-y-6">
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
				<LineItemRow bind:item={lineItems[i]} onremove={() => removeLineItem(i)} />
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
