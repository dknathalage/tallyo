<script lang="ts">
	import { onMount } from 'svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import { session } from '$lib/stores/session.svelte';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';
	import Field from '$lib/components/Field.svelte';

	// Local editable copies so live SSE updates to the store don't clobber typing.
	let name = $state('');
	let email = $state('');
	let phone = $state('');
	let address = $state('');
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
		await businessProfile.save({ name, email, phone, address, defaultCurrency });
		syncFromStore();
		if (businessProfile.error === null) {
			savedNotice = true;
		}
	}
</script>

<section>
	<h1 class="mb-1 text-2xl font-semibold tracking-tight">Business profile</h1>
	<p class="mb-6 text-sm text-gray-500">
		Live value: <span class="font-medium">{businessProfile.profile.name || '—'}</span>
	</p>

	<Card class="max-w-lg">
		<form class="space-y-4" onsubmit={save}>
			<Field label="Name" id="bp-name">
				<input
					id="bp-name"
					type="text"
					bind:value={name}
					disabled={!canManage}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
				/>
			</Field>
			<Field label="Email" id="bp-email">
				<input
					id="bp-email"
					type="email"
					bind:value={email}
					disabled={!canManage}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
				/>
			</Field>
			<Field label="Phone" id="bp-phone">
				<input
					id="bp-phone"
					type="text"
					bind:value={phone}
					disabled={!canManage}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
				/>
			</Field>
			<Field label="Address" id="bp-address">
				<input
					id="bp-address"
					type="text"
					bind:value={address}
					disabled={!canManage}
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
				/>
			</Field>
			<Field label="Default currency" id="bp-currency">
				<input
					id="bp-currency"
					type="text"
					bind:value={defaultCurrency}
					disabled={!canManage}
					placeholder="AUD"
					class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
				/>
			</Field>

			{#if businessProfile.error}
				<p class="text-sm text-red-600">{businessProfile.error}</p>
			{/if}
			{#if savedNotice}
				<p class="text-sm text-green-600">Saved.</p>
			{/if}

			{#if canManage}
				<Button type="submit" loading={businessProfile.saving} disabled={businessProfile.saving}>
					{businessProfile.saving ? 'Saving…' : 'Save'}
				</Button>
			{:else}
				<p class="text-sm text-gray-500">Only an owner or admin can edit business settings.</p>
			{/if}
		</form>
	</Card>
</section>
