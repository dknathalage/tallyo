<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getCatalogItem, updateCatalogItem, deleteCatalogItem, getCatalogItemWithRates, setCatalogItemRate } from '$lib/db/queries/catalog';
	import { getRateTiers } from '$lib/db/queries/rate-tiers';
	import { getEntityHistory } from '$lib/db/queries/audit';
	import { formatCurrency } from '$lib/utils/format';
	import type { AuditLogEntry } from '$lib/types/index.js';
	import CatalogForm from '$lib/components/catalog/CatalogForm.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';

	let itemId = $derived(Number(page.params.id));
	let refreshTrigger = $state(0);
	let item = $derived.by(() => {
		refreshTrigger;
		return getCatalogItem(itemId);
	});
	let itemWithRates = $derived.by(() => {
		refreshTrigger;
		return getCatalogItemWithRates(itemId);
	});
	let tiers = $derived(getRateTiers());

	let history = $derived.by(() => {
		refreshTrigger;
		return getEntityHistory('catalog', itemId);
	});

	let editing = $state(false);
	let showDeleteConfirm = $state(false);

	async function handleUpdate(data: {
		name: string;
		rate: number;
		unit: string;
		category: string;
		sku: string;
		tierRates?: Record<number, number>;
		metadata?: string;
	}) {
		await updateCatalogItem(itemId, data);

		// Save tier rates
		if (data.tierRates) {
			for (const [tierId, rate] of Object.entries(data.tierRates)) {
				await setCatalogItemRate(itemId, Number(tierId), rate);
			}
		}

		editing = false;
		refreshTrigger++;
	}

	async function handleDelete() {
		await deleteCatalogItem(itemId);
		goto(`${base}/catalog`);
	}

	function formatMetadata(meta: string): string {
		try {
			const parsed = JSON.parse(meta);
			return JSON.stringify(parsed, null, 2);
		} catch {
			return meta;
		}
	}

	function formatTimestamp(ts: string): string {
		const d = new Date(ts + 'Z');
		return d.toLocaleString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	function formatAction(action: string): string {
		return action.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function actionColor(action: string): string {
		if (action === 'create') return 'bg-green-100 text-green-800';
		if (action === 'update') return 'bg-blue-100 text-blue-800';
		if (action === 'delete') return 'bg-red-100 text-red-800';
		return 'bg-gray-100 text-gray-800';
	}

	function parseChanges(changesStr: string): Record<string, { old: unknown; new: unknown }> | null {
		try {
			const parsed = JSON.parse(changesStr);
			if (parsed && typeof parsed === 'object' && Object.keys(parsed).length > 0) {
				return parsed;
			}
			return null;
		} catch {
			return null;
		}
	}

	function formatChangeValue(val: unknown): string {
		if (val === null || val === undefined) return '(empty)';
		if (typeof val === 'number') return String(val);
		return String(val) || '(empty)';
	}

	function hasMetadata(meta: string | undefined): boolean {
		if (!meta) return false;
		try {
			const parsed = JSON.parse(meta);
			return Object.keys(parsed).length > 0;
		} catch {
			return meta !== '' && meta !== '{}';
		}
	}
</script>

{#if !item}
	<EmptyState title="Item not found" message="This catalog item does not exist or has been deleted.">
		<a href="{base}/catalog">
			<Button variant="secondary">Back to Catalog</Button>
		</a>
	</EmptyState>
{:else}
	<div class="space-y-6">
		<!-- Header -->
		<div class="flex items-center justify-between">
			<div>
				<a href="{base}/catalog" class="text-sm text-gray-500 hover:text-gray-700">&larr; Back to Catalog</a>
				<h1 class="mt-1 text-2xl font-bold text-gray-900">{item.name}</h1>
			</div>
			<div class="flex gap-2">
				{#if !editing}
					<Button variant="secondary" onclick={() => (editing = true)}>Edit</Button>
				{/if}
				<Button variant="danger" onclick={() => (showDeleteConfirm = true)}>Delete</Button>
			</div>
		</div>

		<!-- Item details / Edit form -->
		<div class="rounded-lg border border-gray-200 bg-white p-6">
			{#if editing}
				<h2 class="mb-4 text-lg font-semibold text-gray-900">Edit Item</h2>
				<CatalogForm initialData={item} onsubmit={handleUpdate} />
			{:else}
				<dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<dt class="text-sm font-medium text-gray-500">Default Rate</dt>
						<dd class="mt-1 text-sm text-gray-900">{formatCurrency(item.rate)}</dd>
					</div>
					<div>
						<dt class="text-sm font-medium text-gray-500">Unit</dt>
						<dd class="mt-1 text-sm text-gray-900">{item.unit || '-'}</dd>
					</div>
					<div>
						<dt class="text-sm font-medium text-gray-500">Category</dt>
						<dd class="mt-1 text-sm text-gray-900">{item.category || '-'}</dd>
					</div>
					<div>
						<dt class="text-sm font-medium text-gray-500">SKU</dt>
						<dd class="mt-1 text-sm text-gray-900">{item.sku || '-'}</dd>
					</div>
				</dl>
			{/if}
		</div>

		<!-- Tier Rates -->
		{#if !editing && tiers.length > 0 && itemWithRates}
			<div class="rounded-lg border border-gray-200 bg-white p-6">
				<h2 class="mb-4 text-lg font-semibold text-gray-900">Tier Rates</h2>
				<dl class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{#each tiers as tier}
						<div>
							<dt class="text-sm font-medium text-gray-500">{tier.name}</dt>
							<dd class="mt-1 text-sm text-gray-900">
								{#if itemWithRates.rates[tier.id] !== undefined}
									{formatCurrency(itemWithRates.rates[tier.id])}
								{:else}
									<span class="text-gray-400">{formatCurrency(item.rate)} (default)</span>
								{/if}
							</dd>
						</div>
					{/each}
				</dl>
			</div>
		{/if}

		<!-- Metadata -->
		{#if !editing && hasMetadata(item.metadata)}
			<div class="rounded-lg border border-gray-200 bg-white p-6">
				<h2 class="mb-4 text-lg font-semibold text-gray-900">Metadata</h2>
				<pre class="overflow-x-auto rounded-lg bg-gray-50 p-4 text-sm text-gray-700">{formatMetadata(item.metadata)}</pre>
			</div>
		{/if}

		<!-- Change History -->
		{#if history.length > 0}
			<div class="rounded-lg border border-gray-200 bg-white p-6">
				<h2 class="mb-4 text-lg font-semibold text-gray-900">Change History</h2>
				<div class="space-y-4">
					{#each history as entry}
						{@const changes = parseChanges(entry.changes)}
						<div class="flex gap-3 border-l-2 border-gray-200 pl-4">
							<div class="flex-1">
								<div class="flex items-center gap-2">
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium {actionColor(entry.action)}">
										{formatAction(entry.action)}
									</span>
									<span class="text-xs text-gray-500">{formatTimestamp(entry.created_at)}</span>
								</div>
								{#if changes}
									<div class="mt-1 space-y-0.5">
										{#each Object.entries(changes) as [field, diff]}
											<p class="text-sm text-gray-600">
												<span class="font-medium">{field}:</span>
												<span class="text-red-600 line-through">{formatChangeValue(diff.old)}</span>
												<span class="text-gray-400">-></span>
												<span class="text-green-700">{formatChangeValue(diff.new)}</span>
											</p>
										{/each}
									</div>
								{:else if entry.context}
									<p class="mt-1 text-sm text-gray-600">{entry.context}</p>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>

	<!-- Delete confirmation -->
	<ConfirmDialog
		open={showDeleteConfirm}
		title="Delete Item"
		message="Are you sure you want to delete {item.name}? This action cannot be undone."
		confirmLabel="Delete"
		confirmVariant="danger"
		onconfirm={handleDelete}
		oncancel={() => (showDeleteConfirm = false)}
	/>
{/if}
