<script lang="ts">

		import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import ClientForm from '$lib/components/client/ClientForm.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	async function handleSubmit(data: { name: string; email: string; phone: string; address: string; metadata: string; payer_id: number | null }) {
		await fetch('/api/clients', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(data)
	});
		goto(`${base}/console/clients`);
	}
</script>

<div class="mx-auto max-w-lg space-y-6">
	<div>
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white">{i18n.t('client.newClient')}</h1>
		<p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{i18n.t('client.addNewClientDesc')}</p>
	</div>

	<div class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800">
		<ClientForm onsubmit={handleSubmit} />
	</div>
</div>
