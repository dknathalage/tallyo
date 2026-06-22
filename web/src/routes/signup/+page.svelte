<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiPost } from '$lib/api/client';
	import { session } from '$lib/stores/session.svelte';
	import type { User, Zone } from '$lib/api/types';

	let businessName = $state('');
	let name = $state('');
	let email = $state('');
	let password = $state('');
	let zone = $state<Zone>('national');
	let error = $state<string | null>(null);
	let submitting = $state(false);

	async function submit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			const user = await apiPost<User>('/api/signup', {
				businessName,
				name,
				email,
				password,
				zone
			});
			// Signup establishes the session; land logged in on the new tenant.
			session.set(user);
			const info = await session.loadSession();
			const first = info?.tenants[0];
			await goto(first ? '/' + first.id + '/' : '/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Sign up failed.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="mx-auto max-w-sm">
	<h1 class="mb-1 text-xl font-semibold">Create your Tallyo account</h1>
	<p class="mb-6 text-sm text-gray-500">Set up your NDIS provider business in one step.</p>

	<form class="space-y-4" onsubmit={submit}>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Business name</span>
			<input
				type="text"
				bind:value={businessName}
				required
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>
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
			<span class="mb-1 block text-sm font-medium">Email</span>
			<input
				type="email"
				bind:value={email}
				required
				autocomplete="email"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">Password</span>
			<input
				type="password"
				bind:value={password}
				required
				minlength="8"
				autocomplete="new-password"
				class="w-full rounded border border-gray-300 px-3 py-2 text-sm"
			/>
			<span class="mt-1 block text-xs text-gray-500">At least 8 characters.</span>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm font-medium">NDIS pricing zone</span>
			<select bind:value={zone} class="w-full rounded border border-gray-300 px-3 py-2 text-sm">
				<option value="national">National</option>
				<option value="remote">Remote</option>
				<option value="very_remote">Very remote</option>
			</select>
			<span class="mt-1 block text-xs text-gray-500">
				Determines the applicable NDIS price caps. You can change this later in Settings.
			</span>
		</label>

		{#if error}
			<p class="text-sm text-red-600">{error}</p>
		{/if}

		<button
			type="submit"
			disabled={submitting}
			class="w-full rounded bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-50"
		>
			{submitting ? 'Creating…' : 'Create account'}
		</button>
	</form>

	<p class="mt-4 text-center text-sm text-gray-500">
		Already have an account? <a href="/login" class="text-gray-900 underline">Sign in</a>
	</p>
</div>
