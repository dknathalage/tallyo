<script lang="ts">
	import { goto } from '$app/navigation';
	import { session } from '$lib/stores/session.svelte';
	import Button from '$lib/components/Button.svelte';

	async function logout(): Promise<void> {
		await session.logout();
		await goto('/login');
	}
</script>

<section>
	<h1 class="mb-1 text-2xl font-semibold tracking-tight">Account</h1>
	<p class="mb-6 text-sm text-gray-500">
		Signed in as <span class="font-medium">{session.user?.email ?? '—'}</span>.
	</p>
	<Button variant="secondary" onclick={logout}>Logout</Button>
</section>
