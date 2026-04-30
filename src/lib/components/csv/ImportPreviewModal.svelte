<script lang="ts">
	import Modal from '$lib/components/shared/Modal.svelte';
	import Button from '$lib/components/shared/Button.svelte';
	import type { ValidationError } from '$lib/csv/types.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let {
		open,
		onclose,
		onconfirm,
		title,
		totalRows,
		validRows,
		skippedDuplicates,
		errors,
		columns,
		previewRows,
		newClients
	}: {
		open: boolean;
		onclose: () => void;
		onconfirm: () => void;
		title: string;
		totalRows: number;
		validRows: number;
		skippedDuplicates: number;
		errors: ValidationError[];
		columns: string[];
		previewRows: Record<string, string>[];
		newClients?: string[];
	} = $props();

	let displayErrors = $derived(errors.slice(0, 20));
	let displayRows = $derived(previewRows.slice(0, 50));
</script>

<Modal {open} {onclose} {title} maxWidth="max-w-4xl">
	<div class="space-y-4">
		<!-- Summary cards -->
		<div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
			<div class="rounded-lg bg-gray-50 dark:bg-gray-800 p-3 text-center">
				<div class="text-2xl font-bold text-gray-900 dark:text-white">{totalRows}</div>
				<div class="text-xs text-gray-500 dark:text-gray-400">{i18n.t('csv.totalRows')}</div>
			</div>
			<div class="rounded-lg bg-green-50 p-3 text-center">
				<div class="text-2xl font-bold text-green-700">{validRows}</div>
				<div class="text-xs text-green-600">{i18n.t('csv.valid')}</div>
			</div>
			<div class="rounded-lg bg-yellow-50 p-3 text-center">
				<div class="text-2xl font-bold text-yellow-700">{skippedDuplicates}</div>
				<div class="text-xs text-yellow-600">{i18n.t('csv.skippedDuplicates')}</div>
			</div>
			<div class="rounded-lg bg-red-50 p-3 text-center">
				<div class="text-2xl font-bold text-red-700">{errors.length}</div>
				<div class="text-xs text-red-600">{i18n.t('csv.errors')}</div>
			</div>
		</div>

		<!-- New clients notice -->
		{#if newClients && newClients.length > 0}
			<div class="rounded-lg border border-blue-200 bg-blue-50 p-3">
				<p class="text-sm font-medium text-blue-800">
					{i18n.t('csv.newClientsAutoCreate')}
				</p>
				<p class="mt-1 text-sm text-blue-700">
					{newClients.join(', ')}
				</p>
			</div>
		{/if}

		<!-- Error list -->
		{#if errors.length > 0}
			<div class="rounded-lg border border-red-200 bg-red-50 p-3">
				<p class="mb-2 text-sm font-medium text-red-800">
					{i18n.t('csv.errorsShowing', { count: String(errors.length), extra: errors.length > 20 ? ', showing first 20' : '' })}
				</p>
				<ul class="space-y-1 text-sm text-red-700">
					{#each displayErrors as err}
						<li>Row {err.row}: {err.field} - {err.message}</li>
					{/each}
				</ul>
			</div>
		{/if}

		<!-- Preview table -->
		{#if displayRows.length > 0}
			<div>
				<p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
					{i18n.t('csv.previewLabel', { count: String(previewRows.length), extra: previewRows.length > 50 ? ', showing first 50' : '' })}
				</p>
				<div class="max-h-64 overflow-auto rounded-lg border border-gray-200 dark:border-gray-700">
					<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700 text-sm">
						<thead class="sticky top-0 bg-gray-50 dark:bg-gray-900">
							<tr>
								{#each columns as col}
									<th class="whitespace-nowrap px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
										{col}
									</th>
								{/each}
							</tr>
						</thead>
						<tbody class="divide-y divide-gray-200 dark:divide-gray-700 bg-white dark:bg-gray-800">
							{#each displayRows as row}
								<tr>
									{#each columns as col}
										<td class="whitespace-nowrap px-3 py-1.5 text-gray-700 dark:text-gray-300">
											{row[col] || ''}
										</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}

		<!-- Footer -->
		<div class="flex items-center justify-end gap-3 border-t border-gray-200 dark:border-gray-700 pt-4">
			<Button variant="secondary" onclick={onclose}>{i18n.t('common.cancel')}</Button>
			<Button disabled={validRows === 0} onclick={onconfirm}>
				{i18n.t('csv.importRows', { count: String(validRows), plural: validRows !== 1 ? 's' : '' })}
			</Button>
		</div>
	</div>
</Modal>
