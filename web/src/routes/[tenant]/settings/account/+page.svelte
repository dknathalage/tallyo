<script lang="ts">
	import { goto } from '$app/navigation';
	import { session } from '$lib/stores/session.svelte';

	async function logout(): Promise<void> {
		await session.logout();
		await goto('/login');
	}
</script>

<section>
	<h1 class="mb-1 text-xl font-semibold">Account</h1>
	<p class="mb-6 text-sm text-gray-500">
		Signed in as <span class="font-medium">{session.user?.email ?? '—'}</span>.
	</p>
	<button
		type="button"
		onclick={logout}
		class="rounded border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100"
	>
		Logout
	</button>
</section>
