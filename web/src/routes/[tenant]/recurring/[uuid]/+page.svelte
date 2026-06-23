<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import { recurring } from '$lib/stores/recurring.svelte';
	import { clients } from '$lib/stores/clients.svelte';
	import { taxRates } from '$lib/stores/taxRates.svelte';
	import type { RecurringFrequency, RecurringLine } from '$lib/api/types';

	// A draft line-item row used by the editor. (Recurring lines are plain
	// code/description/qty/price rows — no catalog/service-date complexity — so a
	// bespoke inline editor is used rather than the shared LineItemsEditor.)
	interface LineRow {
		code: string;
		description: string;
		unit: string;
		quantity: number;
		unitPrice: number;
		taxable: boolean;
	}

	const FREQUENCIES: RecurringFrequency[] = ['weekly', 'monthly', 'quarterly'];

	const idParam = $derived((page.params.uuid ?? 'new'));

	function money(n: number): string {
		const v = Number.isFinite(n) ? n : 0;
		return v.toFixed(2);
	}

	// Form state (shared by create + edit).
	let formName = $state('');
	let formClientId = $state('');
	let formFrequency = $state<RecurringFrequency>('monthly');
	let formNextDue = $state('');
	let formTaxRateId = $state('');
	let formNotes = $state('');
	let formActive = $state(true);
	let lines = $state<LineRow[]>([]);
	let saving = $state(false);
	let formError = $state<string | null>(null);
	let loadError = $state<string | null>(null);

	// Selected tax-rate percent (recurring stores only a number, no taxRateId).
	const selectedTaxRate = $derived.by<number>(() => {
		if (formTaxRateId === '') return 0;
		const tr = taxRates.items.find((t) => t.id === formTaxRateId);
		return tr ? tr.rate : 0;
	});

	function lineAmount(row: LineRow): number {
		return (Number(row.quantity) || 0) * (Number(row.unitPrice) || 0);
	}

	function newLine(): LineRow {
		return { code: '', description: '', unit: '', quantity: 1, unitPrice: 0, taxable: false };
	}

	function addLine(): void {
		lines.push(newLine());
	}

	function removeLine(index: number): void {
		lines.splice(index, 1);
	}

	onMount(() => {
		recurring.ensureSubscribed();
		clients.ensureSubscribed();
		void clients.load();
		taxRates.ensureSubscribed();
		void taxRates.load();
	});

	// Seed the create form, or load the existing template for edit. Re-runs on
	// idParam change (sibling→sibling nav, back/forward, create→edit) so the form
	// state always reflects the current route; the {#key idParam} on the form
	// region remounts the bound inputs alongside this reset.
	function resetForm(): void {
		formName = '';
		formClientId = '';
		formFrequency = 'monthly';
		formTaxRateId = '';
		formNotes = '';
		formActive = true;
		formError = null;
		loadError = null;
	}

	$effect(() => {
		const current = idParam;
		resetForm();
		if (current === 'new') {
			formNextDue = new Date().toISOString().slice(0, 10);
			lines = [newLine()];
		} else {
			formNextDue = '';
			lines = [];
			void loadTemplate(current);
		}
	});

	async function loadTemplate(id: string): Promise<void> {
		loadError = null;
		try {
			const full = await recurring.crud.get(id);
			formName = full.name;
			formClientId = full.clientId === null ? '' : String(full.clientId);
			formFrequency = full.frequency;
			formNextDue = full.nextDue ? full.nextDue.slice(0, 10) : '';
			const matched = taxRates.items.find((t) => t.rate === full.taxRate);
			formTaxRateId = matched ? String(matched.id) : '';
			formNotes = full.notes;
			formActive = full.isActive;
			lines = full.lineItems
				.slice()
				.sort((a, b) => a.sortOrder - b.sortOrder)
				.map((li) => ({
					code: li.code,
					description: li.description,
					unit: li.unit,
					quantity: li.quantity,
					unitPrice: li.unitPrice,
					taxable: li.taxable
				}));
			if (lines.length === 0) lines = [newLine()];
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Failed to load template.';
		}
	}

	// Reconstruct the full RecurringInput. Line items pass through field-for-field
	// (no re-pricing); the relational selects carry the related entity uuid.
	function buildPayload() {
		const items: RecurringLine[] = lines.map((row, i) => ({
			supportItemId: null,
			customItemId: null,
			code: row.code,
			description: row.description,
			unit: row.unit,
			quantity: Number(row.quantity),
			unitPrice: Number(row.unitPrice),
			taxable: row.taxable,
			sortOrder: i
		}));
		return {
			clientId: formClientId === '' ? null : formClientId,
			payerId: null,
			name: formName,
			frequency: formFrequency,
			nextDue: formNextDue,
			lineItems: items,
			taxRate: selectedTaxRate,
			notes: formNotes,
			isActive: formActive
		};
	}

	async function submitForm(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		formError = null;
		if (formClientId === '') {
			formError = 'Please select a client.';
			return;
		}
		saving = true;
		try {
			const payload = buildPayload();
			if (idParam === 'new') {
				const created = await recurring.crud.create(payload);
				await goto(t('/recurring/' + created.id));
			} else {
				await recurring.crud.update(idParam, payload);
				await goto(t('/recurring'));
			}
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to save template.';
		} finally {
			saving = false;
		}
	}
</script>

<div class="space-y-5">
	<a href={t('/recurring')} class="text-sm text-gray-500 hover:text-gray-900">← Back</a>
	<h1 class="text-xl font-semibold">{idParam === 'new' ? 'New template' : 'Edit template'}</h1>

	{#if loadError}
		<p class="text-sm text-red-600">{loadError}</p>
	{/if}

	{#key idParam}
		<form class="space-y-4 rounded border border-gray-200 bg-white p-4" onsubmit={submitForm}>
			<div class="grid grid-cols-2 gap-3">
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Name</span>
					<input
						type="text"
						bind:value={formName}
						required
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
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
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Frequency</span>
					<select
						bind:value={formFrequency}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm capitalize"
					>
						{#each FREQUENCIES as f (f)}
							<option value={f}>{f}</option>
						{/each}
					</select>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Next due</span>
					<input
						type="date"
						bind:value={formNextDue}
						required
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label class="col-span-1">
					<span class="mb-1 block text-sm font-medium">Tax rate</span>
					<select
						bind:value={formTaxRateId}
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					>
						<option value="">— none —</option>
						{#each taxRates.items as tr (tr.id)}
							<option value={String(tr.id)}>{tr.name} ({tr.rate}%)</option>
						{/each}
					</select>
				</label>
				<label class="col-span-1 flex items-end gap-2">
					<input type="checkbox" bind:checked={formActive} class="h-4 w-4" />
					<span class="text-sm font-medium">Active</span>
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
	
			<div>
				<div class="mb-2 flex items-center justify-between">
					<span class="text-sm font-medium">Line items</span>
					<button
						type="button"
						onclick={addLine}
						class="rounded border border-gray-300 px-3 py-1 text-sm hover:bg-gray-50"
					>
						Add line
					</button>
				</div>
	
				<div class="overflow-hidden rounded border border-gray-200">
					<table class="w-full text-sm">
						<thead class="border-b border-gray-200 bg-gray-50 text-left text-gray-500">
							<tr>
								<th class="w-40 px-3 py-2 font-medium">Code</th>
								<th class="px-3 py-2 font-medium">Description</th>
								<th class="w-20 px-3 py-2 font-medium">Qty</th>
								<th class="w-28 px-3 py-2 font-medium">Unit price</th>
								<th class="w-16 px-3 py-2 font-medium">Taxable</th>
								<th class="w-24 px-3 py-2 font-medium text-right">Amount</th>
								<th class="w-12 px-3 py-2"></th>
							</tr>
						</thead>
						<tbody>
							{#each lines as line, i (i)}
								<tr class="border-b border-gray-100 last:border-0">
									<td class="px-3 py-2">
										<input
											type="text"
											bind:value={line.code}
											placeholder="NDIS code"
											class="w-full rounded border border-gray-300 px-2 py-1 font-mono text-xs"
										/>
									</td>
									<td class="px-3 py-2">
										<input
											type="text"
											bind:value={line.description}
											class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
										/>
									</td>
									<td class="px-3 py-2">
										<input
											type="number"
											step="any"
											bind:value={line.quantity}
											class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
										/>
									</td>
									<td class="px-3 py-2">
										<input
											type="number"
											step="any"
											bind:value={line.unitPrice}
											class="w-full rounded border border-gray-300 px-2 py-1 text-sm"
										/>
									</td>
									<td class="px-3 py-2 text-center">
										<input type="checkbox" bind:checked={line.taxable} class="h-4 w-4" />
									</td>
									<td class="px-3 py-2 text-right whitespace-nowrap">
										{money(lineAmount(line))}
									</td>
									<td class="px-3 py-2 text-right">
										<button
											type="button"
											onclick={() => removeLine(i)}
											class="text-red-600 hover:underline"
											aria-label="Remove line"
										>
											✕
										</button>
									</td>
								</tr>
							{:else}
								<tr>
									<td colspan="7" class="px-3 py-4 text-center text-gray-500"> No line items. </td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
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
					{saving ? 'Saving…' : idParam === 'new' ? 'Create template' : 'Save changes'}
				</button>
				<a
					href={t('/recurring')}
					class="rounded border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
				>
					Cancel
				</a>
			</div>
		</form>
	{/key}
</div>
