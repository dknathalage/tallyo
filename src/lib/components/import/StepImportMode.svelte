<script lang="ts">
	import Button from '$lib/components/shared/Button.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let {
		onselect
	}: {
		onselect: (mode: 'insert_only' | 'upsert') => void;
	} = $props();

	let mode: 'insert_only' | 'upsert' = $state('upsert');
</script>

<div class="space-y-4">
	<p class="text-sm text-gray-600 dark:text-gray-300">{i18n.t('importWizard.importModeDesc')}</p>

	<div class="space-y-3">
		<!-- Insert Only -->
		<label
			class="flex cursor-pointer items-start gap-3 rounded-lg border p-4 transition-colors {mode === 'insert_only' ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/30' : 'border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600'}"
		>
			<input
				type="radio"
				bind:group={mode}
				value="insert_only"
				class="mt-0.5 h-4 w-4 cursor-pointer text-primary-600 focus:ring-primary-500"
			/>
			<div>
				<div class="font-medium text-gray-900 dark:text-white">{i18n.t('importWizard.insertOnly')}</div>
				<div class="mt-1 text-sm text-gray-500 dark:text-gray-400">
					{i18n.t('importWizard.insertOnlyDesc')}
				</div>
			</div>
		</label>

		<!-- Upsert -->
		<label
			class="flex cursor-pointer items-start gap-3 rounded-lg border p-4 transition-colors {mode === 'upsert' ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/30' : 'border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600'}"
		>
			<input
				type="radio"
				bind:group={mode}
				value="upsert"
				class="mt-0.5 h-4 w-4 cursor-pointer text-primary-600 focus:ring-primary-500"
			/>
			<div>
				<div class="font-medium text-gray-900 dark:text-white">{i18n.t('importWizard.insertUpdate')}</div>
				<div class="mt-1 text-sm text-gray-500 dark:text-gray-400">
					{i18n.t('importWizard.insertUpdateDesc')}
				</div>
			</div>
		</label>
	</div>

	<!-- Footer -->
	<div class="flex justify-end border-t border-gray-200 dark:border-gray-700 pt-4">
		<Button onclick={() => onselect(mode)}>
			{i18n.t('common.next')}
		</Button>
	</div>
</div>
