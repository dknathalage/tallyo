<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { createCatalogItem } from '$lib/db/queries/catalog';
	import CatalogForm from '$lib/components/catalog/CatalogForm.svelte';

	async function handleSubmit(data: { name: string; rate: number; unit: string; category: string; sku: string }) {
		await createCatalogItem(data);
		goto(`${base}/catalog`);
	}
</script>

<div class="mx-auto max-w-lg space-y-6">
	<div>
		<h1 class="text-2xl font-bold text-gray-900">New Catalog Item</h1>
		<p class="mt-1 text-sm text-gray-500">Add a new item to your catalog.</p>
	</div>

	<div class="rounded-lg border border-gray-200 bg-white p-6">
		<CatalogForm onsubmit={handleSubmit} />
	</div>
</div>
