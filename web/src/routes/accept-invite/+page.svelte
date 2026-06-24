<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { apiGet, apiPost } from '$lib/api/client';
	import type { InviteInfo } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import Card from '$lib/components/Card.svelte';
	import Field from '$lib/components/Field.svelte';
	import Receipt from '@lucide/svelte/icons/receipt';

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

<div class="mx-auto flex min-h-screen max-w-sm flex-col justify-center px-4 py-12">
	<a href="/login" class="mb-6 flex items-center justify-center gap-2">
		<span class="flex size-8 items-center justify-center rounded-lg bg-brand-700 text-onbrand">
			<Receipt class="size-5" aria-hidden="true" />
		</span>
		<span class="text-xl font-semibold tracking-tight text-brand-700">Tallyo</span>
	</a>

	<Card>
		<h1 class="mb-6 text-xl font-semibold tracking-tight">Accept your invitation</h1>

		{#if loading}
			<p class="text-sm text-gray-500">Loading invite…</p>
		{:else if invite !== null}
			<p class="mb-4 text-sm text-gray-600">
				Set a password for <span class="font-medium">{invite.email}</span>
				({invite.role}).
			</p>
			<form class="space-y-4" onsubmit={submit}>
				<Field label="Your name" id="name">
					<input
						id="name"
						type="text"
						bind:value={name}
						required
						autocomplete="name"
						class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
					/>
				</Field>
				<Field label="Password" id="password">
					<input
						id="password"
						type="password"
						bind:value={password}
						required
						autocomplete="new-password"
						class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
					/>
				</Field>

				{#if error}
					<p class="text-sm text-red-600" role="alert">{error}</p>
				{/if}

				<Button type="submit" loading={submitting} class="w-full">Accept invite</Button>
			</form>
		{:else}
			<p class="text-sm text-red-600" role="alert">{error ?? 'Invite not found.'}</p>
		{/if}
	</Card>
</div>
