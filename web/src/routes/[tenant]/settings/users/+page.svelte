<script lang="ts">
	import { session } from '$lib/stores/session.svelte';
	import { apiPost, tenantPath } from '$lib/api/client';
	import type { InviteCreated, Role } from '$lib/api/types';

	// owner/admin may manage users; member is read-only.
	const canManage = $derived(session.isManager);

	let inviteEmail = $state('');
	let inviteRole = $state<Role>('member');
	let acceptUrl = $state<string | null>(null);
	let inviteError = $state<string | null>(null);
	let inviting = $state(false);

	async function createInvite(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		inviteError = null;
		acceptUrl = null;
		inviting = true;
		try {
			const created = await apiPost<InviteCreated>(tenantPath('invites'), {
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

<section>
	<h1 class="mb-1 text-xl font-semibold">Users</h1>
	<p class="mb-6 text-sm text-gray-500">Generate an invite link for a new team member.</p>

	{#if canManage}
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
	{:else}
		<p class="text-sm text-gray-500">Only an owner or admin can manage users.</p>
	{/if}
</section>
