<script lang="ts">
	import { onMount, untrack } from 'svelte';

	import { today, formatCurrency } from '$lib/utils/format.js';
	import type { Client, Estimate, EstimateLineItem, KeyValuePair, TaxRate, RateTier } from '$lib/types/index.js';
	import { parseSnapshot } from '$lib/utils/snapshot.js';
	import type { PartySnapshot } from '$lib/utils/snapshot.js';
	import Button from '$lib/components/shared/Button.svelte';
	import KeyValueEditor from '$lib/components/shared/KeyValueEditor.svelte';
	import CurrencySelect from '$lib/components/shared/CurrencySelect.svelte';
	import LineItemRow from '$lib/components/invoice/LineItemRow.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	const {
		initialData,
		initialLineItems,
		nextEstimateNumber,
		onsubmit
	}: {
		initialData?: Estimate;
		initialLineItems?: EstimateLineItem[];
		nextEstimateNumber?: string;
		onsubmit: (
			data: {
				estimate_number: string;
				client_id: number;
				date: string;
				valid_until: string;
				subtotal: number;
				tax_rate: number;
				tax_rate_id: number | null;
				tax_amount: number;
				total: number;
				notes: string;
				status: string;
				currency_code: string;
				business_snapshot: string;
				client_snapshot: string;
				payer_snapshot: string;
			},
			lineItems: { description: string; quantity: number; rate: number; amount: number; sort_order: number; notes: string }[]
		) => void;
	} = $props();

	let clients: Client[] = $state([]);
	let estimateNumber = $state(untrack(() => initialData?.estimate_number ?? ''));
	let clientId: number | '' = $state(untrack(() => initialData?.client_id ?? ''));
	let date = $state(untrack(() => initialData?.date ?? today()));
	let validUntil = $state(untrack(() => initialData?.valid_until ?? today()));
	let taxRates = $state<TaxRate[]>([]);
	let selectedTaxRateId = $state<number | null>(untrack(() => initialData?.tax_rate_id ?? null));
	const taxRate = $derived.by(() => {
		if (selectedTaxRateId !== null) {
			const tr = taxRates.find((r) => r.id === selectedTaxRateId);
			return tr ? tr.rate : 0;
		}
		return 0;
	});
	let showNewTaxRate = $state(false);
	let newTaxRateName = $state('');
	let newTaxRateValue = $state(0);
	let notes = $state(untrack(() => initialData?.notes ?? ''));
	let status = $state(untrack(() => initialData?.status ?? 'draft'));
	let currencyCode = $state(untrack(() => initialData?.currency_code ?? ''));

	let lineItems = $state<{ description: string; quantity: number; rate: number; amount: number; unit?: string | undefined; notes?: string | undefined }[]>(
		[{ description: '', quantity: 1, rate: 0, amount: 0, unit: undefined, notes: '' }]
	);

	$effect(() => {
		if (initialData) {
			clientId = initialData.client_id;
			date = initialData.date;
			validUntil = initialData.valid_until;
			selectedTaxRateId = initialData.tax_rate_id;
			notes = initialData.notes;
			status = initialData.status;
			currencyCode = initialData.currency_code;
		}
		if (initialLineItems) {
			lineItems = initialLineItems.map((li) => ({
				description: li.description,
				quantity: li.quantity,
				rate: li.rate,
				amount: li.amount,
				notes: li.notes
			}));
		}
	});

	let tiers = $state<RateTier[]>([]);
	let selectedClient = $state<Client | null>(null);

	onMount(async () => {
		const res = await fetch('/api/rate-tiers');
		tiers = await res.json();
	});

	$effect(() => {
		const id = clientId;
		if (id) {
			void fetch(`/api/clients/${id}`).then(r => r.json()).then(c => { selectedClient = c; });
		} else {
			selectedClient = null;
		}
	});

	const activeTierId = $derived(selectedClient?.pricing_tier_id ?? null);
	const activeTierName = $derived(tiers.find(t => t.id === activeTierId)?.name ?? null);

	const subtotal = $derived(
		Math.round(lineItems.reduce((sum, item) => sum + item.amount, 0) * 100) / 100
	);
	const taxAmount = $derived(Math.round(subtotal * (taxRate / 100) * 100) / 100);
	const total = $derived(Math.round((subtotal + taxAmount) * 100) / 100);

	// --- Snapshot helpers ---
	function parseMetadata(metaStr?: string): KeyValuePair[] {
		try {
			const obj = JSON.parse(metaStr ?? '{}');
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

	// Business snapshot (read-only, always from current profile)
	let businessSnapshot: PartySnapshot = $state({ name: '', email: '', phone: '', address: '', metadata: {} });

	// Client metadata (editable on estimate)
	let clientMetadataPairs: KeyValuePair[] = $state([]);

	// Payer info (editable on estimate)
	let payerName = $state('');
	let payerEmail = $state('');
	let payerPhone = $state('');
	let payerAddress = $state('');
	let payerMetadataPairs: KeyValuePair[] = $state([]);

	function applyDefaultTaxRate() {
		if (selectedTaxRateId !== null || taxRates.length === 0) return;
		const defaultRate = taxRates.find((r: TaxRate) => r.is_default === 1) ?? taxRates[0];
		if (defaultRate) selectedTaxRateId = defaultRate.id;
	}

	// eslint-disable-next-line @typescript-eslint/no-explicit-any -- profile shape comes from JSON API
	function buildBusinessSnapshot(profile: any): PartySnapshot {
		return {
			name: profile.name ?? '',
			email: profile.email ?? '',
			phone: profile.phone ?? '',
			address: profile.address ?? '',
			metadata: (() => { try { return JSON.parse(profile.metadata ?? '{}'); } catch { return {}; } })()
		};
	}

	function loadEditSnapshots(data: Estimate) {
		const cs = parseSnapshot(data.client_snapshot);
		clientMetadataPairs = Object.entries(cs.metadata).map(([key, value]) => ({ key, value }));
		const ps = parseSnapshot(data.payer_snapshot);
		payerName = ps.name;
		payerEmail = ps.email;
		payerPhone = ps.phone;
		payerAddress = ps.address;
		payerMetadataPairs = Object.entries(ps.metadata).map(([key, value]) => ({ key, value }));
	}

	// Initialize clients, estimate number, business snapshot, and edit-mode snapshots
	onMount(async () => {
		const [clientsRes, settingsRes] = await Promise.all([
			fetch('/api/clients'),
			fetch('/api/settings')
		]);
		clients = await clientsRes.json();
		const settings = await settingsRes.json();
		taxRates = settings.taxRates ?? [];
		applyDefaultTaxRate();
		if (!initialData) {
			estimateNumber = nextEstimateNumber ?? '';
			const profile = settings.profile;
			if (profile && !currencyCode) {
				currencyCode = profile.default_currency ?? 'USD';
			}
		}
		if (!currencyCode) currencyCode = 'USD';

		if (settings.profile) businessSnapshot = buildBusinessSnapshot(settings.profile);
		if (initialData) loadEditSnapshots(initialData);
	});

	// Auto-populate client metadata and payer when client changes (new estimates only)
	$effect(() => {
		const id = clientId;
		if (!id || initialData) return;

		void fetch(`/api/clients/${id}`).then(r => r.json()).then(async (client) => {
			if (!client) return;
			clientMetadataPairs = parseMetadata(client.metadata);

			if (client.payer_id) {
				const payer = await fetch(`/api/payers/${client.payer_id}`).then(r => r.json());
				if (payer) {
					payerName = payer.name;
					payerEmail = payer.email;
					payerPhone = payer.phone;
					payerAddress = payer.address;
					payerMetadataPairs = parseMetadata(payer.metadata);
				} else {
					payerName = payerEmail = payerPhone = payerAddress = '';
					payerMetadataPairs = [];
				}
			} else {
				payerName = payerEmail = payerPhone = payerAddress = '';
				payerMetadataPairs = [];
			}
		});
	});

	async function addTaxRate() {
		const name = newTaxRateName.trim();
		const rate = newTaxRateValue;
		if (!name) return;
		newTaxRateName = '';
		newTaxRateValue = 0;
		const res = await fetch('/api/tax-rates', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name, rate })
		});
		const { id: newId } = await res.json();
		const settingsRes = await fetch('/api/settings');
		const settings = await settingsRes.json();
		taxRates = settings.taxRates ?? [];
		selectedTaxRateId = newId;
		showNewTaxRate = false;
	}

	function addLineItem() {
		lineItems.push({ description: '', quantity: 1, rate: 0, amount: 0, unit: undefined, notes: '' });
	}

	function removeLineItem(index: number) {
		lineItems.splice(index, 1);
	}

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();

		if (!clientId) return;

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
				estimate_number: estimateNumber,
				client_id: clientId,
				date,
				valid_until: validUntil,
				subtotal,
				tax_rate: taxRate,
				tax_rate_id: selectedTaxRateId,
				tax_amount: taxAmount,
				total,
				notes,
				status,
				currency_code: currencyCode,
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
		<div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 p-4">
			<h3 class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('invoice.from')}</h3>
			<p class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{businessSnapshot.name}</p>
			{#if businessSnapshot.email}<p class="text-sm text-gray-500 dark:text-gray-400">{businessSnapshot.email}</p>{/if}
			{#if businessSnapshot.phone}<p class="text-sm text-gray-500 dark:text-gray-400">{businessSnapshot.phone}</p>{/if}
			{#if businessSnapshot.address}<p class="whitespace-pre-line text-sm text-gray-500 dark:text-gray-400">{businessSnapshot.address}</p>{/if}
			{#if Object.keys(businessSnapshot.metadata).length > 0}
				<div class="mt-1 space-y-0.5">
					{#each Object.entries(businessSnapshot.metadata) as [key, value]}
						<p class="text-sm text-gray-500 dark:text-gray-400"><span class="font-medium text-gray-700 dark:text-gray-300">{key}:</span> {value}</p>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	<!-- Header fields -->
	<fieldset class="border-0 p-0 m-0">
		<legend class="sr-only">{i18n.t('a11y.estimateDetails')}</legend>
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div>
				<label for="estimate-number" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('estimate.estimateNumber')}</label>
				<input
					id="estimate-number"
					type="text"
					bind:value={estimateNumber}
					readonly
					class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-900 px-3 py-2 text-sm text-gray-900 dark:text-white focus:outline-none"
				/>
			</div>

			<div>
				<label for="client" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('invoice.client')}</label>
				<select
					id="client"
					bind:value={clientId}
					required
					class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				>
					<option value="" disabled>{i18n.t('invoice.selectClient')}</option>
					{#each clients as client}
						<option value={client.id}>{client.name}</option>
					{/each}
				</select>
				{#if activeTierName}
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{i18n.t('invoice.pricing', { tier: activeTierName })}</p>
				{/if}
			</div>

			<div>
				<label for="date" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('invoice.date')}</label>
				<input
					id="date"
					type="date"
					bind:value={date}
					required
					class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				/>
			</div>

			<div>
				<label for="valid-until" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('estimate.validUntil')}</label>
				<input
					id="valid-until"
					type="date"
					bind:value={validUntil}
					required
					class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				/>
			</div>

			<div>
				<label for="currency" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('invoice.currency')}</label>
				<div class="mt-1">
					<CurrencySelect id="currency" bind:value={currencyCode} />
				</div>
			</div>
		</div>
	</fieldset>

	<!-- Line Items -->
	<fieldset class="border-0 p-0 m-0">
		<legend class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('invoice.lineItems')}</legend>

		<!-- Header -->
		<div class="mb-2 flex items-center gap-3 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400" aria-hidden="true">
			<div class="flex-1">{i18n.t('invoice.description')}</div>
			<div class="w-24">{i18n.t('invoice.qty')}</div>
			<div class="w-28">{i18n.t('invoice.rate')}</div>
			<div class="w-28 text-right">{i18n.t('invoice.amount')}</div>
			<div class="w-8"></div>
		</div>

		<div class="space-y-2">
			{#each lineItems as _item, i (i)}
				{#if lineItems[i]}
					<LineItemRow bind:item={lineItems[i]} onremove={() => removeLineItem(i)} tierId={activeTierId} {currencyCode} />
				{/if}
			{/each}
		</div>

		<button
			type="button"
			onclick={addLineItem}
			class="mt-3 cursor-pointer text-sm font-medium text-primary-600 hover:text-primary-700"
		>
			{i18n.t('invoice.addLineItem')}
		</button>
	</fieldset>

	<!-- Client Metadata -->
	{#if clientId}
		<fieldset class="border-0 p-0 m-0">
			<legend class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('invoice.clientAdditionalFields')}</legend>
			<KeyValueEditor bind:pairs={clientMetadataPairs} addLabel={i18n.t('common.addField')} />
		</fieldset>
	{/if}

	<!-- Payer / Bill-To -->
	{#if clientId}
		<fieldset class="rounded-lg border border-gray-200 dark:border-gray-700 p-4 m-0">
			<legend class="text-sm font-medium text-gray-700 dark:text-gray-300 px-1">{i18n.t('invoice.billToPayer')}</legend>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<div>
					<label for="payer-name" class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.name')}</label>
					<input
						id="payer-name"
						type="text"
						bind:value={payerName}
						placeholder={i18n.t('invoice.payerNamePlaceholder')}
						class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
					/>
				</div>
				<div>
					<label for="payer-email" class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.email')}</label>
					<input
						id="payer-email"
						type="email"
						bind:value={payerEmail}
						placeholder={i18n.t('invoice.payerEmailPlaceholder')}
						class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
					/>
				</div>
				<div>
					<label for="payer-phone" class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.phone')}</label>
					<input
						id="payer-phone"
						type="tel"
						bind:value={payerPhone}
						placeholder={i18n.t('invoice.payerPhonePlaceholder')}
						class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
					/>
				</div>
				<div>
					<label for="payer-address" class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.address')}</label>
					<input
						id="payer-address"
						type="text"
						bind:value={payerAddress}
						placeholder={i18n.t('invoice.payerAddressPlaceholder')}
						class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
					/>
				</div>
			</div>
			<div class="mt-3">
				<span class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('common.additionalFields')}</span>
				<div class="mt-1">
					<KeyValueEditor bind:pairs={payerMetadataPairs} addLabel={i18n.t('common.addField')} />
				</div>
			</div>
		</fieldset>
	{/if}

	<!-- Tax and totals -->
	<div class="flex justify-end">
		<div class="w-72 space-y-2">
			<div class="flex justify-between text-sm">
				<span class="text-gray-600 dark:text-gray-300">{i18n.t('invoice.subtotal')}</span>
				<span class="font-medium text-gray-900 dark:text-white">{formatCurrency(subtotal, currencyCode)}</span>
			</div>

			<div class="flex items-center justify-between gap-3 text-sm">
				<label for="est-tax-rate-select" class="text-gray-600 dark:text-gray-300">{i18n.t('invoice.taxRate')}</label>
				<div class="flex items-center gap-2">
					<select
						id="est-tax-rate-select"
						bind:value={selectedTaxRateId}
						class="rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-2 py-1 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none"
					>
						{#each taxRates as tr}
							<option value={tr.id}>{tr.name} ({tr.rate}%)</option>
						{/each}
					</select>
					<button
						type="button"
						onclick={() => (showNewTaxRate = !showNewTaxRate)}
						class="text-xs text-primary-600 hover:text-primary-700 cursor-pointer"
					>+ New</button>
				</div>
			</div>
			{#if showNewTaxRate}
				<div class="flex items-center gap-2 text-sm">
					<input type="text" bind:value={newTaxRateName} placeholder="Name" class="flex-1 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-2 py-1 text-sm text-gray-900 dark:text-white" />
					<input type="number" bind:value={newTaxRateValue} min="0" step="any" placeholder="%" class="w-16 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-2 py-1 text-sm text-gray-900 dark:text-white" />
					<button type="button" onclick={() => void addTaxRate()} class="text-xs bg-primary-600 text-white px-2 py-1 rounded cursor-pointer">Add</button>
				</div>
			{/if}

			<div class="flex justify-between text-sm">
				<span class="text-gray-600 dark:text-gray-300">{i18n.t('invoice.tax')}</span>
				<span class="font-medium text-gray-900 dark:text-white">{formatCurrency(taxAmount, currencyCode)}</span>
			</div>

			<div class="flex justify-between border-t border-gray-200 dark:border-gray-700 pt-2 text-base">
				<span class="font-semibold text-gray-900 dark:text-white">{i18n.t('invoice.total')}</span>
				<span class="font-semibold text-gray-900 dark:text-white">{formatCurrency(total, currencyCode)}</span>
			</div>
		</div>
	</div>

	<!-- Notes -->
	<div>
		<label for="notes" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('invoice.notes')}</label>
		<textarea
			id="notes"
			bind:value={notes}
			rows={3}
			placeholder={i18n.t('invoice.notesPlaceholder')}
			class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
		></textarea>
	</div>

	<!-- Actions -->
	<div class="flex justify-end gap-3">
		<Button type="submit">
			{initialData ? i18n.t('common.saveChanges') : i18n.t('estimate.newEstimate')}
		</Button>
	</div>
</form>
