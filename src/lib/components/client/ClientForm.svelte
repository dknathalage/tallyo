<script lang="ts">
	import type { Client } from '$lib/types';
	import Button from '$lib/components/shared/Button.svelte';

	let {
		initialData,
		onsubmit
	}: {
		initialData?: Client;
		onsubmit: (data: { name: string; email: string; phone: string; address: string }) => void;
	} = $props();

	let name = $state(initialData?.name ?? '');
	let email = $state(initialData?.email ?? '');
	let phone = $state(initialData?.phone ?? '');
	let address = $state(initialData?.address ?? '');

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		onsubmit({ name, email, phone, address });
	}
</script>

<form onsubmit={handleSubmit} class="space-y-4">
	<div>
		<label for="name" class="block text-sm font-medium text-gray-700">Name <span class="text-red-500">*</span></label>
		<input
			id="name"
			type="text"
			bind:value={name}
			required
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="Client name"
		/>
	</div>

	<div>
		<label for="email" class="block text-sm font-medium text-gray-700">Email</label>
		<input
			id="email"
			type="email"
			bind:value={email}
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="client@example.com"
		/>
	</div>

	<div>
		<label for="phone" class="block text-sm font-medium text-gray-700">Phone</label>
		<input
			id="phone"
			type="tel"
			bind:value={phone}
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="(555) 123-4567"
		/>
	</div>

	<div>
		<label for="address" class="block text-sm font-medium text-gray-700">Address</label>
		<textarea
			id="address"
			bind:value={address}
			rows={3}
			class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
			placeholder="Street address, city, state, zip"
		></textarea>
	</div>

	<div class="flex justify-end gap-3 pt-2">
		<Button variant="secondary" onclick={() => history.back()}>Cancel</Button>
		<Button type="submit">{initialData ? 'Save Changes' : 'Create Client'}</Button>
	</div>
</form>
