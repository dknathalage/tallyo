<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiPost } from '$lib/api/client';
	import type { User } from '$lib/api/types';

	let email = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let submitting = $state(false);

	async function submit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			const user = await apiPost<User>('/api/auth/login', { email, password });
			if (user === null) {
				error = 'Invalid email or password.';
				return;
			}
			await goto('/settings');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Invalid email or password.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="mx-auto max-w-sm">
	<h1 class="mb-6 text-xl font-semibold">Sign in to Tallyo</h1>

	<form class="space-y-4" onsubmit={submit}>
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
				autocomplete="current-password"
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
			{submitting ? 'Signing in…' : 'Sign in'}
		</button>
	</form>
</div>
