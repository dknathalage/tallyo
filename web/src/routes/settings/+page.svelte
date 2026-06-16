<script lang="ts">
	import { onMount } from 'svelte';
	import { businessProfile } from '$lib/stores/businessProfile.svelte';
	import { session } from '$lib/stores/session.svelte';
	import { apiPost } from '$lib/api/client';
	import type { InviteCreated, Role, Zone } from '$lib/api/types';

	// Local editable copies so live SSE updates to the store don't clobber typing.
	let name = $state('');
	let email = $state('');
	let phone = $state('');
	let address = $state('');
	let zone = $state<Zone>('national');
	let defaultCurrency = $state('');
	let savedNotice = $state(false);

	// Invite section.
	let inviteEmail = $state('');
	let inviteRole = $state<Role>('member');
	let acceptUrl = $state<string | null>(null);
	let inviteError = $state<string | null>(null);
	let inviting = $state(false);

	// owner/admin may edit settings + manage users; member is read-only.
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

	async function createInvite(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		inviteError = null;
		acceptUrl = null;
		inviting = true;
		try {
			const created = await apiPost<InviteCreated>('/api/invites', {
				email: inviteEmail,
				role: inviteRole
			});
			acceptUrl = created?.acceptUrl ?? null;
			inviteEmail = '';
		} catch (err) {
			inviteError = err instanceof Error ? err.message : 'Failed to create invite.';
		} finally {
			inviting = false;
		}
	}
</script>

<div class="space-y-10">
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
				<span class="mb-1 block text-sm font-medium">NDIS pricing zone</span>
				<select
					bind:value={zone}
					disabled={!canManage}
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm disabled:bg-gray-100"
				>
					<option value="national">National</option>
					<option value="remote">Remote</option>
					<option value="very_remote">Very remote</option>
				</select>
				<span class="mt-1 block text-xs text-gray-500">
					Determines which NDIS price caps apply to your support-item lines.
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

	{#if canManage}
		<section class="border-t border-gray-200 pt-8">
			<h2 class="mb-1 text-lg font-semibold">Invite a user</h2>
			<p class="mb-4 text-sm text-gray-500">Generate an invite link for a new team member.</p>

			<form class="flex max-w-lg flex-wrap items-end gap-3" onsubmit={createInvite}>
				<label class="flex-1">
					<span class="mb-1 block text-sm font-medium">Email</span>
					<input
						type="email"
						bind:value={inviteEmail}
						required
						class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
					/>
				</label>
				<label>
					<span class="mb-1 block text-sm font-medium">Role</span>
					<select bind:value={inviteRole} class="rounded border border-gray-300 px-3 py-2 text-sm">
						<option value="member">Member</option>
						<option value="admin">Admin</option>
						<option value="owner">Owner</option>
					</select>
				</label>
				<button
					type="submit"
					disabled={inviting}
					class="rounded bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{inviting ? 'Creating…' : 'Invite'}
				</button>
			</form>

			{#if inviteError}
				<p class="mt-3 text-sm text-red-600">{inviteError}</p>
			{/if}
			{#if acceptUrl}
				<div class="mt-4 max-w-lg rounded border border-gray-200 bg-white p-3">
					<p class="mb-1 text-sm font-medium">Invite link</p>
					<input
						type="text"
						readonly
						value={acceptUrl}
						class="w-full rounded border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-xs"
					/>
				</div>
			{/if}
		</section>
	{/if}
</div>
