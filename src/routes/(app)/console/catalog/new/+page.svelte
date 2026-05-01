<script lang="ts">
	
		import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import CatalogForm from '$lib/components/catalog/CatalogForm.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	async function handleSubmit(data: { name: string; rate: number; unit: string; category: string; sku: string }) {
		await fetch('/api/catalog', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
		void goto(resolve('/(app)/console/catalog'));
	}
</script>

<div class="mx-auto max-w-lg space-y-6">
	<div>
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('catalog.newCatalogItem')}</h1>
		<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{i18n.t('catalog.addItemDesc')}</p>
	</div>

	<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
		<CatalogForm onsubmit={handleSubmit} />
	</div>
</div>
