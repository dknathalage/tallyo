<script lang="ts">
	import { goto } from '$app/navigation';
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { PageData } from './$types';
	import { formatCurrency } from '$lib/utils/format';
	import CatalogForm from '$lib/components/catalog/CatalogForm.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import ConfirmDialog from '$lib/components/shared/ConfirmDialog.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { apiFetch } from '$lib/utils/api.js';

	const { data }: { data: PageData } = $props();

	const item = $derived(data.item);
	const itemWithRates = $derived(data.itemWithRates);
	const tiers = $derived(data.rateTiers);
	const history = $derived(data.auditHistory);

	let editing = $state(false);
	let showDeleteConfirm = $state(false);

	async function handleUpdate(updates: {
		name: string;
		rate: number;
		unit: string;
		category: string;
		sku: string;
		tierRates?: Record<number, number> | undefined;
		metadata?: string | undefined;
	}) {
		await fetch(`/api/catalog/${item.id}`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(updates)
		});

		editing = false;
		await invalidateAll();
	}

	async function handleDelete() {
		const res = await apiFetch(`/api/catalog/${item.id}`, { method: 'DELETE' });
		if (res.ok) void goto(resolve('/(app)/console/catalog'));
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
	<EmptyState title={i18n.t('catalog.itemNotFound')} message={i18n.t('catalog.itemNotFoundMessage')}>
		<a href={resolve('/(app)/console/catalog')}>
			<Button variant="secondary">{i18n.t('catalog.backToCatalog')}</Button>
		</a>
	</EmptyState>
{:else}
	<div class="space-y-6">
		<!-- Header -->
		<div class="flex items-center justify-between">
			<div>
				<a href={resolve('/(app)/console/catalog')} class="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200">&larr; {i18n.t('catalog.backToCatalog')}</a>
				<h1 class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">{item.name}</h1>
			</div>
			<div class="flex gap-2">
				{#if !editing}
					<Button variant="secondary" onclick={() => (editing = true)}>{i18n.t('common.edit')}</Button>
				{/if}
				<Button variant="danger" onclick={() => (showDeleteConfirm = true)}>{i18n.t('common.delete')}</Button>
			</div>
		</div>

		<!-- Item details / Edit form -->
		<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
			{#if editing}
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('catalog.editItem')}</h2>
				<CatalogForm initialData={item} onsubmit={handleUpdate} />
			{:else}
				<dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('catalog.defaultRate')}</dt>
						<dd class="mt-1 text-sm text-gray-900 dark:text-white">{formatCurrency(item.rate)}</dd>
					</div>
					<div>
						<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('catalog.unit')}</dt>
						<dd class="mt-1 text-sm text-gray-900 dark:text-white">{item.unit || '-'}</dd>
					</div>
					<div>
						<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('catalog.category')}</dt>
						<dd class="mt-1 text-sm text-gray-900 dark:text-white">{item.category || '-'}</dd>
					</div>
					<div>
						<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{i18n.t('catalog.sku')}</dt>
						<dd class="mt-1 text-sm text-gray-900 dark:text-white">{item.sku || '-'}</dd>
					</div>
				</dl>
			{/if}
		</div>

		<!-- Tier Rates -->
		{#if !editing && tiers.length > 0 && itemWithRates}
			<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('catalog.tierRates')}</h2>
				<dl class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{#each tiers as tier}
						{@const tierRate = itemWithRates.rates[tier.id]}
						<div>
							<dt class="text-sm font-medium text-gray-500 dark:text-gray-400">{tier.name}</dt>
							<dd class="mt-1 text-sm text-gray-900 dark:text-white">
								{#if tierRate !== undefined}
									{formatCurrency(tierRate)}
								{:else}
									<span class="text-gray-400 dark:text-gray-500">{formatCurrency(item.rate)} {i18n.t('common.default')}</span>
								{/if}
							</dd>
						</div>
					{/each}
				</dl>
			</div>
		{/if}

		<!-- Metadata -->
		{#if !editing && hasMetadata(item.metadata)}
			<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('catalog.metadata')}</h2>
				<pre class="overflow-x-auto rounded-lg bg-gray-50 p-4 text-sm text-gray-700 dark:bg-gray-900 dark:text-gray-300">{formatMetadata(item.metadata)}</pre>
			</div>
		{/if}

		<!-- Change History -->
		{#if history.length > 0}
			<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
				<h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{i18n.t('catalog.changeHistory')}</h2>
				<div class="space-y-4">
					{#each history as entry}
						{@const changes = parseChanges(entry.changes)}
						<div class="flex gap-3 border-l-2 border-gray-200 pl-4 dark:border-gray-700">
							<div class="flex-1">
								<div class="flex items-center gap-2">
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium {actionColor(entry.action)}">
										{formatAction(entry.action)}
									</span>
									<span class="text-xs text-gray-500 dark:text-gray-400">{formatTimestamp(entry.created_at)}</span>
								</div>
								{#if changes}
									<div class="mt-1 space-y-0.5">
										{#each Object.entries(changes) as [field, diff]}
											<p class="text-sm text-gray-600 dark:text-gray-300">
												<span class="font-medium">{field}:</span>
												<span class="text-red-600 line-through">{formatChangeValue(diff.old)}</span>
												<span class="text-gray-400 dark:text-gray-500">-></span>
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
		title={i18n.t('catalog.deleteConfirmTitle')}
		message={i18n.t('catalog.deleteConfirmMessage', { name: item.name })}
		confirmLabel={i18n.t('common.delete')}
		confirmVariant="danger"
		onconfirm={handleDelete}
		oncancel={() => (showDeleteConfirm = false)}
	/>
{/if}
