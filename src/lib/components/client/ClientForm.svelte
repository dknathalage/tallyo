<script lang="ts">
	import type { Client, Payer, KeyValuePair } from '$lib/types';
	import Button from '$lib/components/shared/Button.svelte';
	import KeyValueEditor from '$lib/components/shared/KeyValueEditor.svelte';
	import { getPayers } from '$lib/db/queries/payers';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let {
		initialData,
		onsubmit
	}: {
		initialData?: Client;
		onsubmit: (data: { name: string; email: string; phone: string; address: string; metadata: string; payer_id: number | null }) => void;
	} = $props();

	let name = $state(initialData?.name ?? '');
	let email = $state(initialData?.email ?? '');
	let phone = $state(initialData?.phone ?? '');
	let address = $state(initialData?.address ?? '');
	let payerId: number | null = $state(initialData?.payer_id ?? null);
	let payers: Payer[] = $state([]);
	let metadataPairs: KeyValuePair[] = $state(parseMetadata(initialData?.metadata));

	function parseMetadata(metaStr?: string): KeyValuePair[] {
		try {
			const obj = JSON.parse(metaStr || '{}');
			return Object.entries(obj).map(([key, value]) => ({ key, value: String(value) }));
		} catch {
			return [];
		}
	}

	$effect(() => {
		payers = getPayers();
	});

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		const metaObj: Record<string, string> = {};
		for (const pair of metadataPairs) {
			if (pair.key.trim()) {
				metaObj[pair.key.trim()] = pair.value;
			}
		}
		onsubmit({ name, email, phone, address, metadata: JSON.stringify(metaObj), payer_id: payerId });
	}
</script>

<form onsubmit={handleSubmit} class="space-y-4">
	<fieldset class="space-y-4 border-0 p-0 m-0">
		<legend class="sr-only">{i18n.t('a11y.contactInfo')}</legend>
		<div>
			<label for="name" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('client.name')} <span class="text-red-500">*</span></label>
			<input
				id="name"
				type="text"
				bind:value={name}
				required
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder={i18n.t('client.clientNamePlaceholder')}
			/>
		</div>

		<div>
			<label for="email" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('client.email')}</label>
			<input
				id="email"
				type="email"
				bind:value={email}
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder={i18n.t('client.emailPlaceholder')}
			/>
		</div>

		<div>
			<label for="phone" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('client.phone')}</label>
			<input
				id="phone"
				type="tel"
				bind:value={phone}
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder={i18n.t('client.phonePlaceholder')}
			/>
		</div>

		<div>
			<label for="address" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('client.address')}</label>
			<textarea
				id="address"
				bind:value={address}
				rows={3}
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
				placeholder={i18n.t('client.addressPlaceholder')}
			></textarea>
		</div>

		<div>
			<label for="payer" class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('client.billToPayer')}</label>
			<select
				id="payer"
				bind:value={payerId}
				class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			>
				<option value={null}>{i18n.t('common.none')}</option>
				{#each payers as payer}
					<option value={payer.id}>{payer.name}</option>
				{/each}
			</select>
		</div>
	</fieldset>

	<fieldset class="border-0 p-0 m-0">
		<legend class="block text-sm font-medium text-gray-700 dark:text-gray-300">{i18n.t('common.additionalFields')}</legend>
		<div class="mt-1">
			<KeyValueEditor bind:pairs={metadataPairs} addLabel={i18n.t('common.addField')} />
		</div>
	</fieldset>

	<div class="flex justify-end gap-3 pt-2">
		<Button variant="secondary" onclick={() => history.back()}>{i18n.t('common.cancel')}</Button>
		<Button type="submit">{initialData ? i18n.t('common.saveChanges') : i18n.t('client.createClient')}</Button>
	</div>
</form>
