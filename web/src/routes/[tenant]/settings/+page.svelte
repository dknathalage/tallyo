<script lang="ts">
	import { onMount } from 'svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import { session } from '$lib/stores/session.svelte';
	import type { Zone } from '$lib/api/types';

	// Local editable copies so live SSE updates to the store don't clobber typing.
	let name = $state('');
	let email = $state('');
	let phone = $state('');
	let address = $state('');
	let zone = $state<Zone>('national');
	let defaultCurrency = $state('');
	let savedNotice = $state(false);

	// owner/admin may edit settings; member is read-only.
	const canManage = $derived(session.isManager);

	function syncFromStore(): void {
		const p = businessProfile.profile;
		name = p.name;
		email = p.email;
		phone = p.phone;
		address = p.address;
		zone = p.zone;
		defaultCurrency = p.defaultCurrency;
	}

	onMount(() => {
		businessProfile.subscribe();
		void (async () => {
			await businessProfile.load();
			syncFromStore();
		})();
	});

	async function save(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		savedNotice = false;
		await businessProfile.save({ name, email, phone, address, zone, defaultCurrency });
		syncFromStore();
		if (businessProfile.error === null) {
			savedNotice = true;
		}
	}
</script>

<section>
	<h1 class="mb-1 text-xl font-semibold">Business profile</h1>
	<p class="mb-6 text-sm text-gray-500">
		Live value: <span class="font-medium">{businessProfile.profile.name || '—'}</span>
	</p>

	<form class="max-w-lg space-y-4" onsubmit={save}>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Name</span>
			<input
				type="text"
				bind:value={name}
				disabled={!canManage}
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Email</span>
			<input
				type="email"
				bind:value={email}
				disabled={!canManage}
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Phone</span>
			<input
				type="text"
				bind:value={phone}
				disabled={!canManage}
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Address</span>
			<input
				type="text"
				bind:value={address}
				disabled={!canManage}
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">NDIS pricing zone (optional)</span>
			<select
				bind:value={zone}
				disabled={!canManage}
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
			>
				<option value="">None (not NDIS)</option>
				<option value="national">National</option>
				<option value="remote">Remote</option>
				<option value="very_remote">Very remote</option>
			</select>
			<span class="mt-1 block text-xs text-gray-500">
				NDIS-only. Determines which NDIS price caps apply to your support-item lines. Leave it at
				the default if you don't invoice against the NDIS price catalogue.
			</span>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Default currency</span>
			<input
				type="text"
				bind:value={defaultCurrency}
				disabled={!canManage}
				placeholder="AUD"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
			/>
		</label>

		{#if businessProfile.error}
			<p class="text-sm text-red-600">{businessProfile.error}</p>
		{/if}
		{#if savedNotice}
			<p class="text-sm text-green-600">Saved.</p>
		{/if}

		{#if canManage}
			<button
				type="submit"
				disabled={businessProfile.saving}
				class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{businessProfile.saving ? 'Saving…' : 'Save'}
			</button>
		{:else}
			<p class="text-sm text-gray-500">Only an owner or admin can edit business settings.</p>
		{/if}
	</form>
</section>
