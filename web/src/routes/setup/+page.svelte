<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiPost } from '$lib/api/client';

	let email = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let submitting = $state(false);

	async function submit(e: SubmitEvent): Promise<void> {
		e.preventDefault();
		error = null;
		submitting = true;
		try {
			await apiPost('/api/setup', { email, password });
			await goto('/login');
		} catch (err) {
			error = err instanceof Error ? err.message : 'setup failed';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="mx-auto max-w-sm">
	<h1 class="mb-1 text-xl font-semibold">Create the owner account</h1>
	<p class="mb-6 text-sm text-gray-500">Set up the first user for this Tallyo instance.</p>

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
			{submitting ? 'Creating…' : 'Create account'}
		</button>
	</form>
</div>
