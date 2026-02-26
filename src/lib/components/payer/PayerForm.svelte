<script lang="ts">
	import type { Payer, KeyValuePair } from '$lib/types';
	import Button from '$lib/components/shared/Button.svelte';
	import KeyValueEditor from '$lib/components/shared/KeyValueEditor.svelte';

	let {
		initialData,
		onsubmit
	}: {
		initialData?: Payer;
		onsubmit: (data: { name: string; email: string; phone: string; address: string; metadata: string }) => void;
	} = $props();

	let name = $state(initialData?.name ?? '');
	let email = $state(initialData?.email ?? '');
	let phone = $state(initialData?.phone ?? '');
	let address = $state(initialData?.address ?? '');

	// Parse metadata from JSON string into pairs for editor
	let metadataPairs: KeyValuePair[] = $state(parseMetadata(initialData?.metadata));

	function parseMetadata(metaStr?: string): KeyValuePair[] {
		try {
			const obj = JSON.parse(metaStr || '{}');
			return Object.entries(obj).map(([key, value]) => ({ key, value: String(value) }));
		} catch {
			return [];
		}
	}

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		const metaObj: Record<string, string> = {};
		for (const pair of metadataPairs) {
			if (pair.key.trim()) {
				metaObj[pair.key.trim()] = pair.value;
			}
		}
		onsubmit({ name, email, phone, address, metadata: JSON.stringify(metaObj) });
	}
</script>

<form onsubmit={handleSubmit} class="space-y-4">
	<div>
		<label for="payer-name" class="block text-sm font-medium text-gray-700 dark:text-gray-300">Name <span class="text-red-500">*</span></label>
		<input
			id="payer-name"
			type="text"
			bind:value={name}
			required
			class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="Payer name"
		/>
	</div>

	<div>
		<label for="payer-email" class="block text-sm font-medium text-gray-700 dark:text-gray-300">Email</label>
		<input
			id="payer-email"
			type="email"
			bind:value={email}
			class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="payer@example.com"
		/>
	</div>

	<div>
		<label for="payer-phone" class="block text-sm font-medium text-gray-700 dark:text-gray-300">Phone</label>
		<input
			id="payer-phone"
			type="tel"
			bind:value={phone}
			class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="(555) 123-4567"
		/>
	</div>

	<div>
		<label for="payer-address" class="block text-sm font-medium text-gray-700 dark:text-gray-300">Address</label>
		<textarea
			id="payer-address"
			bind:value={address}
			rows={3}
			class="mt-1 block w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="Street address, city, state, zip"
		></textarea>
	</div>

	<div>
		<label class="block text-sm font-medium text-gray-700 dark:text-gray-300">Additional Fields</label>
		<div class="mt-1">
			<KeyValueEditor bind:pairs={metadataPairs} addLabel="Add Field" />
		</div>
	</div>

	<div class="flex justify-end gap-3 pt-2">
		<Button type="submit">{initialData ? 'Save Changes' : 'Create Payer'}</Button>
	</div>
</form>
