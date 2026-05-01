<script lang="ts">
	import KeyValueEditor from '$lib/components/shared/KeyValueEditor.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import type { KeyValuePair } from '$lib/types/index.js';

	// eslint-disable-next-line prefer-const -- bindable props require `let`
	let {
		payerName = $bindable(),
		payerEmail = $bindable(),
		payerPhone = $bindable(),
		payerAddress = $bindable(),
		payerMetadataPairs = $bindable()
	}: {
		payerName: string;
		payerEmail: string;
		payerPhone: string;
		payerAddress: string;
		payerMetadataPairs: KeyValuePair[];
	} = $props();
</script>

<fieldset class="rounded-lg border border-gray-200 dark:border-gray-700 p-4 m-0">
	<legend class="text-sm font-medium text-gray-700 dark:text-gray-300 px-1">{i18n.t('invoice.billToPayer')}</legend>
	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
		<div>
			<label for="payer-name" class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.name')}</label>
			<input id="payer-name" type="text" bind:value={payerName} placeholder={i18n.t('invoice.payerNamePlaceholder')} class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20" />
		</div>
		<div>
			<label for="payer-email" class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.email')}</label>
			<input id="payer-email" type="email" bind:value={payerEmail} placeholder={i18n.t('invoice.payerEmailPlaceholder')} class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20" />
		</div>
		<div>
			<label for="payer-phone" class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.phone')}</label>
			<input id="payer-phone" type="tel" bind:value={payerPhone} placeholder={i18n.t('invoice.payerPhonePlaceholder')} class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20" />
		</div>
		<div>
			<label for="payer-address" class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('client.address')}</label>
			<input id="payer-address" type="text" bind:value={payerAddress} placeholder={i18n.t('invoice.payerAddressPlaceholder')} class="mt-1 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20" />
		</div>
	</div>
	<div class="mt-3">
		<span class="block text-xs font-medium text-gray-500 dark:text-gray-400">{i18n.t('common.additionalFields')}</span>
		<div class="mt-1">
			<KeyValueEditor bind:pairs={payerMetadataPairs} addLabel={i18n.t('common.addField')} />
		</div>
	</div>
</fieldset>
