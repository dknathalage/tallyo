<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { apiGet, apiPost } from '$lib/api/client';
	import type { InviteInfo } from '$lib/api/types';

	let token = $state('');
	let invite = $state<InviteInfo | null>(null);
	let name = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let loading = $state(true);
	let submitting = $state(false);

	onMount(() => {
		token = page.url.searchParams.get('token') ?? '';
		void loadInvite();
	});

	async function loadInvite(): Promise<void> {
		loading = true;
		error = null;
		if (token === '') {
			error = 'Missing invite token.';
			loading = false;
			return;
		}
		try {
			invite = await apiGet<InviteInfo>(`/api/invites/${encodeURIComponent(token)}`);
			if (invite === null) {
				error = 'This invite is invalid or has expired.';
			}
		} catch {
			error = 'This invite is invalid or has expired.';
		} finally {
			loading = false;
		}
	}

	async function submit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			await apiPost(`/api/invites/${encodeURIComponent(token)}/accept`, { name, password });
			await goto('/login');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to accept invite.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="mx-auto max-w-sm">
	<h1 class="mb-6 text-xl font-semibold">Accept your invitation</h1>

	{#if loading}
		<p class="text-sm text-gray-500">Loading invite…</p>
	{:else if invite !== null}
		<p class="mb-4 text-sm text-gray-600">
			Set a password for <span class="font-medium">{invite.email}</span>
			({invite.role}).
		</p>
		<form class="space-y-4" onsubmit={submit}>
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Your name</span>
				<input
					type="text"
					bind:value={name}
					required
					autocomplete="name"
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>
			<label class="block">
				<span class="mb-1 block text-sm font-medium">Password</span>
				<input
					type="password"
					bind:value={password}
					required
					autocomplete="new-password"
					class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
				/>
			</label>

			{#if error}
				<p class="text-sm text-red-600">{error}</p>
			{/if}

			<button
				type="submit"
				disabled={submitting}
				class="w-full rounded bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{submitting ? 'Saving…' : 'Accept invite'}
			</button>
		</form>
	{:else}
		<p class="text-sm text-red-600">{error ?? 'Invite not found.'}</p>
	{/if}
</div>
