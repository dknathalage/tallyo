<script lang="ts">
	import { session } from '$lib/stores/session.svelte';
	import { apiPost, tenantPath } from '$lib/api/client';
	import type { InviteCreated, Role } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import Field from '$lib/components/Field.svelte';

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
	<h1 class="mb-1 text-2xl font-semibold tracking-tight">Users</h1>
	<p class="mb-6 text-sm text-gray-500">Generate an invite link for a new team member.</p>

	{#if canManage}
		<form class="flex max-w-lg flex-wrap items-end gap-3" onsubmit={createInvite}>
			<div class="flex-1">
				<Field label="Email" id="invite-email" required>
					<input
						id="invite-email"
						type="email"
						bind:value={inviteEmail}
						required
						class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
					/>
				</Field>
			</div>
			<Field label="Role" id="invite-role">
				<select
					id="invite-role"
					bind:value={inviteRole}
					class="rounded-lg border border-gray-300 px-3 py-2 text-sm"
				>
					<option value="member">Member</option>
					<option value="admin">Admin</option>
					<option value="owner">Owner</option>
				</select>
			</Field>
			<Button type="submit" loading={inviting} disabled={inviting}>
				{inviting ? 'Creating…' : 'Invite'}
			</Button>
		</form>

		{#if inviteError}
			<p class="mt-3 text-sm text-red-600">{inviteError}</p>
		{/if}
		{#if acceptUrl}
			<div class="mt-4 max-w-lg rounded-xl border border-gray-200 bg-white p-3 shadow-sm">
				<p class="mb-1 text-sm font-medium">Invite link</p>
				<input
					type="text"
					readonly
					value={acceptUrl}
					class="w-full rounded-lg border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-xs"
				/>
			</div>
		{/if}
	{:else}
		<p class="text-sm text-gray-500">Only an owner or admin can manage users.</p>
	{/if}
</section>
