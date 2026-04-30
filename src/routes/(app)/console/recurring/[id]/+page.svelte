<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { invalidateAll } from '$app/navigation';
	import { base } from '$app/paths';
	import type { PageData } from './$types';
	import type { RecurringTemplate, RecurringFrequency } from '$lib/types/index.js';
	import type { Client } from '$lib/types/index.js';
	import Button from '$lib/components/shared/Button.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let { data }: { data: PageData } = $props();

	let isNew = $derived(!data.template);
	let templateId = $derived(data.template?.id ?? null);

	let template: RecurringTemplate | null = $state(untrack(() => data.template ?? null));
	let clients: Client[] = $state(untrack(() => data.clients ?? []));

	// Form fields
	let name = $state('');
	let clientId = $state<number | ''>('');
	let frequency = $state<RecurringFrequency>('monthly');
	let nextDue = $state(new Date().toISOString().slice(0, 10));
	let taxRate = $state(0);
	let notes = $state('');
	let isActive = $state(true);

	// Line items
	interface FormLineItem {
		description: string;
		quantity: number;
		rate: number;
		amount: number;
		notes: string;
		sort_order: number;
	}
	let lineItems: FormLineItem[] = $state([
		{ description: '', quantity: 1, rate: 0, amount: 0, notes: '', sort_order: 0 }
	]);

	let saving = $state(false);
	let errors: Record<string, string> = $state({});
	let showDeleteConfirm = $state(false);
	let creatingInvoice = $state(false);

	let subtotal = $derived(lineItems.reduce((sum, li) => sum + li.amount, 0));
	let taxAmount = $derived(subtotal * (taxRate / 100));
	let total = $derived(subtotal + taxAmount);

	// Initialize form fields from server data
	$effect(() => {
		const t = template;
		if (t) {
			name = t.name;
			clientId = t.client_id ?? '';
			frequency = t.frequency;
			nextDue = t.next_due;
			taxRate = t.tax_rate;
			notes = t.notes ?? '';
			isActive = t.is_active === 1;
			try {
				const parsed = JSON.parse(t.line_items);
				if (Array.isArray(parsed) && parsed.length > 0) {
					lineItems = parsed.map((li: FormLineItem, i: number) => ({ ...li, sort_order: i }));
				}
			} catch {
				lineItems = [{ description: '', quantity: 1, rate: 0, amount: 0, notes: '', sort_order: 0 }];
			}
		}
	});

	function updateLineItem(index: number, field: keyof FormLineItem, value: string | number) {
		const items = [...lineItems];
		const existing = items[index];
		if (!existing) return;
		const item: FormLineItem = { ...existing, [field]: value };
		if (field === 'quantity' || field === 'rate') {
			item.amount = item.quantity * item.rate;
		}
		items[index] = item;
		lineItems = items;
	}

	function addLineItem() {
		lineItems = [...lineItems, { description: '', quantity: 1, rate: 0, amount: 0, notes: '', sort_order: lineItems.length }];
	}

	function removeLineItem(index: number) {
		lineItems = lineItems.filter((_, i) => i !== index).map((li, i) => ({ ...li, sort_order: i }));
		if (lineItems.length === 0) {
			lineItems = [{ description: '', quantity: 1, rate: 0, amount: 0, notes: '', sort_order: 0 }];
		}
	}

	function validate(): boolean {
		const errs: Record<string, string> = {};
		if (!name.trim()) errs['name'] = i18n.t('validation.required');
		if (!nextDue) errs['nextDue'] = i18n.t('validation.required');
		errors = errs;
		return Object.keys(errs).length === 0;
	}

	async function handleSave() {
		if (!validate()) return;
		saving = true;
		try {
			const data = {
				client_id: clientId === '' ? 0 : Number(clientId),
				name: name.trim(),
				frequency,
				next_due: nextDue,
				line_items: JSON.stringify(lineItems),
				tax_rate: taxRate,
				notes: notes.trim(),
				is_active: isActive ? 1 : 0
			};
			if (isNew) {
				const res = await fetch('/api/recurring', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(data)
				});
				const { id: newId } = await res.json();
				goto(`${base}/console/recurring/${newId}`);
			} else if (templateId) {
				await fetch(`/api/recurring/${templateId}`, {
					method: 'PUT',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(data)
				});
				await invalidateAll();
			}
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!templateId) return;
		await fetch(`/api/recurring/${templateId}`, { method: 'DELETE' });
		goto(`${base}/console/recurring`);
	}

	async function handleCreateInvoice() {
		if (!templateId) return;
		creatingInvoice = true;
		try {
			const res = await fetch(`/api/recurring/${templateId}`, { method: 'PATCH' });
			const { invoiceId } = await res.json();
			goto(`${base}/console/invoices/${invoiceId}`);
		} finally {
			creatingInvoice = false;
		}
	}

	function formatCurrencySimple(amount: number): string {
		return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(amount);
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex flex-wrap items-center justify-between gap-4">
		<div>
			<a href="{base}/console/recurring" class="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200">
				← {i18n.t('recurring.backToTemplates')}
			</a>
			<h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">
				{isNew ? i18n.t('recurring.newTemplate') : (template?.name ?? i18n.t('recurring.editTemplate'))}
			</h1>
		</div>
		{#if !isNew && template}
			<div class="flex items-center gap-2">
				{#if template.is_active}
					<Button onclick={handleCreateInvoice} disabled={creatingInvoice}>
						{creatingInvoice ? 'Creating...' : i18n.t('recurring.createFromTemplate')}
					</Button>
				{/if}
				<Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>
					{i18n.t('recurring.deleteTemplate')}
				</Button>
			</div>
		{/if}
	</div>

	{#if !isNew && templateId && !template}
		<p class="text-gray-600 dark:text-gray-300">{i18n.t('recurring.notFound')}</p>
	{:else}
		<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
			<div class="space-y-4">
				<!-- Name -->
				<div>
					<label for="tmpl-name" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
						{i18n.t('recurring.templateName')} <span class="text-red-500">*</span>
					</label>
					<input
						id="tmpl-name"
						type="text"
						bind:value={name}
						placeholder={i18n.t('recurring.templateNamePlaceholder')}
						class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white {errors['name'] ? 'border-red-500' : ''}"
					/>
					{#if errors['name']}<p class="mt-1 text-xs text-red-500">{errors['name']}</p>{/if}
				</div>

				<!-- Client + Frequency row -->
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<label for="tmpl-client" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
							{i18n.t('recurring.clientName')}
						</label>
						<select
							id="tmpl-client"
							bind:value={clientId}
							class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
						>
							<option value="">{i18n.t('common.none')}</option>
							{#each clients as c}
								<option value={c.id}>{c.name}</option>
							{/each}
						</select>
					</div>
					<div>
						<label for="tmpl-freq" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
							{i18n.t('recurring.frequency')}
						</label>
						<select
							id="tmpl-freq"
							bind:value={frequency}
							class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
						>
							<option value="weekly">{i18n.t('recurring.weekly')}</option>
							<option value="monthly">{i18n.t('recurring.monthly')}</option>
							<option value="quarterly">{i18n.t('recurring.quarterly')}</option>
						</select>
					</div>
				</div>

				<!-- Next Due + Tax Rate row -->
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<label for="tmpl-next-due" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
							{i18n.t('recurring.nextDue')} <span class="text-red-500">*</span>
						</label>
						<input
							id="tmpl-next-due"
							type="date"
							bind:value={nextDue}
							class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white {errors['nextDue'] ? 'border-red-500' : ''}"
						/>
						{#if errors['nextDue']}<p class="mt-1 text-xs text-red-500">{errors['nextDue']}</p>{/if}
					</div>
					<div>
						<label for="tmpl-tax" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
							{i18n.t('recurring.taxRate')}
						</label>
						<input
							id="tmpl-tax"
							type="number"
							min="0"
							max="100"
							step="0.01"
							bind:value={taxRate}
							class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
						/>
					</div>
				</div>

				<!-- Active toggle -->
				<div class="flex items-center gap-2">
					<input
						id="tmpl-active"
						type="checkbox"
						bind:checked={isActive}
						class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
					/>
					<label for="tmpl-active" class="text-sm font-medium text-gray-700 dark:text-gray-300">
						{i18n.t('recurring.isActive')}
					</label>
				</div>

				<!-- Line Items -->
				<div>
					<h3 class="text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('recurring.lineItems')}</h3>
					<div class="mt-2 space-y-2">
						{#each lineItems as li, i}
							<div class="grid grid-cols-12 gap-2">
								<div class="col-span-5">
									<input
										type="text"
										value={li.description}
										oninput={(e) => updateLineItem(i, 'description', e.currentTarget.value)}
										placeholder={i18n.t('invoice.description')}
										class="block w-full rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
									/>
								</div>
								<div class="col-span-2">
									<input
										type="number"
										value={li.quantity}
										oninput={(e) => updateLineItem(i, 'quantity', parseFloat(e.currentTarget.value) || 0)}
										placeholder={i18n.t('invoice.qty')}
										min="0"
										step="any"
										class="block w-full rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
									/>
								</div>
								<div class="col-span-2">
									<input
										type="number"
										value={li.rate}
										oninput={(e) => updateLineItem(i, 'rate', parseFloat(e.currentTarget.value) || 0)}
										placeholder={i18n.t('invoice.rate')}
										min="0"
										step="any"
										class="block w-full rounded-lg border border-gray-300 px-3 py-1.5 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
									/>
								</div>
								<div class="col-span-2">
									<input
										type="number"
										value={li.amount}
										readonly
										class="block w-full rounded-lg border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-600 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-400"
									/>
								</div>
								<div class="col-span-1 flex items-center justify-center">
									<button
										type="button"
										onclick={() => removeLineItem(i)}
										aria-label={i18n.t('a11y.removeLineItem')}
										class="cursor-pointer rounded p-1 text-gray-400 hover:text-red-500"
									>
										<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
											<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
										</svg>
									</button>
								</div>
							</div>
						{/each}
					</div>
					<button
						type="button"
						onclick={addLineItem}
						class="mt-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
					>
						{i18n.t('invoice.addLineItem')}
					</button>
				</div>

				<!-- Totals -->
				<div class="ml-auto w-full max-w-xs space-y-1 border-t border-gray-200 pt-3 dark:border-gray-700">
					<div class="flex justify-between text-sm text-gray-600 dark:text-gray-400">
						<span>{i18n.t('invoice.subtotal')}</span>
						<span>{formatCurrencySimple(subtotal)}</span>
					</div>
					{#if taxRate > 0}
						<div class="flex justify-between text-sm text-gray-600 dark:text-gray-400">
							<span>Tax ({taxRate}%)</span>
							<span>{formatCurrencySimple(taxAmount)}</span>
						</div>
					{/if}
					<div class="flex justify-between text-sm font-semibold text-gray-900 dark:text-white">
						<span>{i18n.t('invoice.total')}</span>
						<span>{formatCurrencySimple(total)}</span>
					</div>
				</div>

				<!-- Notes -->
				<div>
					<label for="tmpl-notes" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
						{i18n.t('recurring.notes')}
					</label>
					<textarea
						id="tmpl-notes"
						bind:value={notes}
						rows="3"
						placeholder={i18n.t('invoice.notesPlaceholder')}
						class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-gray-600 dark:bg-gray-700 dark:text-white"
					></textarea>
				</div>

				<!-- Actions -->
				<div class="flex justify-end gap-3 border-t border-gray-200 pt-4 dark:border-gray-700">
					<Button variant="secondary" onclick={() => goto(`${base}/console/recurring`)}>{i18n.t('common.cancel')}</Button>
					<Button onclick={handleSave} disabled={saving}>
						{saving ? i18n.t('common.loading') : (isNew ? i18n.t('recurring.createTemplate') : i18n.t('recurring.updateTemplate'))}
					</Button>
				</div>
			</div>
		</div>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => (showDeleteConfirm = false)} title={i18n.t('recurring.deleteConfirmTitle')}>
	<p class="text-sm text-gray-600 dark:text-gray-300">
		{i18n.t('recurring.deleteConfirmMessage')}
	</p>
	<div class="mt-4 flex justify-end gap-3">
		<Button variant="secondary" size="sm" onclick={() => (showDeleteConfirm = false)}>{i18n.t('common.cancel')}</Button>
		<Button variant="danger" size="sm" onclick={handleDelete}>{i18n.t('common.delete')}</Button>
	</div>
</Modal>
