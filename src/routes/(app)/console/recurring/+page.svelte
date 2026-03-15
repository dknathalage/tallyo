<script lang="ts">
	import type { RecurringTemplate } from '$lib/types/index.js';
	import type { PageData } from './$types';
	import { formatDate } from '$lib/utils/format.js';
	import Button from '$lib/components/shared/Button.svelte';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import Modal from '$lib/components/shared/Modal.svelte';
	import { goto } from '$app/navigation';
	import { invalidateAll } from '$app/navigation';
	import { base } from '$app/paths';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let { data }: { data: PageData } = $props();

	let showAll = $state(false);
	let showDeleteConfirm = $state(false);
	let templateToDelete: RecurringTemplate | null = $state(null);
	let creatingFrom: number | null = $state(null);

	let templates = $derived(
		showAll ? data.templates : data.templates.filter((t: RecurringTemplate) => t.is_active)
	);

	async function handleCreateFromTemplate(template: RecurringTemplate) {
		creatingFrom = template.id;
		try {
			const res = await fetch(`/api/recurring/${template.id}`, { method: 'PATCH' });
			const { invoiceId } = await res.json();
			await invalidateAll();
			goto(`${base}/console/invoices/${invoiceId}`);
		} finally {
			creatingFrom = null;
		}
	}

	async function handleToggleActive(template: RecurringTemplate) {
		const newActive = template.is_active ? 0 : 1;
		await fetch(`/api/recurring/${template.id}`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				client_id: template.client_id,
				name: template.name,
				frequency: template.frequency,
				next_due: template.next_due,
				line_items: template.line_items,
				tax_rate: template.tax_rate,
				notes: template.notes,
				is_active: newActive
			})
		});
		await invalidateAll();
	}

	async function handleDelete() {
		if (!templateToDelete) return;
		await fetch(`/api/recurring/${templateToDelete.id}`, { method: 'DELETE' });
		templateToDelete = null;
		showDeleteConfirm = false;
		await invalidateAll();
	}

	function confirmDelete(template: RecurringTemplate) {
		templateToDelete = template;
		showDeleteConfirm = true;
	}

	function frequencyLabel(freq: string): string {
		switch (freq) {
			case 'weekly': return i18n.t('recurring.weekly');
			case 'monthly': return i18n.t('recurring.monthly');
			case 'quarterly': return i18n.t('recurring.quarterly');
			default: return freq;
		}
	}

	let today = new Date().toISOString().slice(0, 10);

	function isDue(template: RecurringTemplate): boolean {
		return template.next_due <= today && template.is_active === 1;
	}
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex flex-wrap items-center justify-between gap-4">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('recurring.title')}</h1>
		<div class="flex items-center gap-3">
			<label class="flex cursor-pointer items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
				<input
					type="checkbox"
					bind:checked={showAll}
					class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
				/>
				Show inactive
			</label>
			<Button onclick={() => goto(`${base}/console/recurring/new`)}>{i18n.t('recurring.newTemplate')}</Button>
		</div>
	</div>

	{#if templates.length === 0}
		<EmptyState title={i18n.t('recurring.noTemplates')} message={i18n.t('recurring.noTemplatesMessage')}>
			<Button onclick={() => goto(`${base}/console/recurring/new`)}>{i18n.t('recurring.newTemplate')}</Button>
		</EmptyState>
	{:else}
		<div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<caption class="sr-only">{i18n.t('recurring.title')}</caption>
				<thead class="bg-gray-50 dark:bg-gray-900">
					<tr>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">Name</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('recurring.clientName')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('recurring.frequency')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{i18n.t('recurring.nextDue')}</th>
						<th scope="col" class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">Status</th>
						<th scope="col" class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">Actions</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each templates as template}
						<tr class="hover:bg-gray-50 dark:hover:bg-gray-700 {isDue(template) ? 'bg-amber-50 dark:bg-amber-900/20' : ''}">
							<td
								class="cursor-pointer px-4 py-3 text-sm font-medium text-primary-600"
								onclick={() => goto(`${base}/console/recurring/${template.id}`)}
							>
								{template.name}
								{#if isDue(template)}
									<span class="ml-2 inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-900/40 dark:text-amber-300">
										Due
									</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-sm text-gray-900 dark:text-white">
								{template.client_name ?? '—'}
							</td>
							<td class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
								{frequencyLabel(template.frequency)}
							</td>
							<td class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
								{formatDate(template.next_due)}
							</td>
							<td class="px-4 py-3 text-sm">
								{#if template.is_active}
									<span class="inline-flex items-center rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800 dark:bg-green-900/40 dark:text-green-300">
										{i18n.t('recurring.active')}
									</span>
								{:else}
									<span class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-gray-700 dark:text-gray-400">
										{i18n.t('recurring.inactive')}
									</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-right">
								<div class="flex items-center justify-end gap-2">
									{#if template.is_active}
										<Button
											size="sm"
											onclick={() => handleCreateFromTemplate(template)}
											disabled={creatingFrom === template.id}
										>
											{creatingFrom === template.id ? 'Creating...' : i18n.t('recurring.createFromTemplate')}
										</Button>
									{/if}
									<Button
										variant="secondary"
										size="sm"
										onclick={() => goto(`${base}/console/recurring/${template.id}`)}
									>
										{i18n.t('common.edit')}
									</Button>
									<Button
										variant="secondary"
										size="sm"
										onclick={() => handleToggleActive(template)}
									>
										{template.is_active ? i18n.t('recurring.deactivate') : i18n.t('recurring.activate')}
									</Button>
									<Button
										variant="danger"
										size="sm"
										onclick={() => confirmDelete(template)}
									>
										{i18n.t('common.delete')}
									</Button>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal open={showDeleteConfirm} onclose={() => { showDeleteConfirm = false; templateToDelete = null; }} title={i18n.t('recurring.deleteConfirmTitle')}>
	<p class="text-sm text-gray-600 dark:text-gray-300">
		{i18n.t('recurring.deleteConfirmMessage')}
	</p>
	<div class="mt-4 flex justify-end gap-3">
		<Button variant="secondary" size="sm" onclick={() => { showDeleteConfirm = false; templateToDelete = null; }}>{i18n.t('common.cancel')}</Button>
		<Button variant="danger" size="sm" onclick={handleDelete}>{i18n.t('common.delete')}</Button>
	</div>
</Modal>
