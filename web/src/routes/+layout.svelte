<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { session } from '$lib/stores/session.svelte';
	import { theme } from '$lib/stores/theme.svelte';

	let { children } = $props();

	let ready = $state(false);

	const PUBLIC_PATHS = ['/login', '/signup', '/accept-invite'];

	function isPublic(path: string): boolean {
		return PUBLIC_PATHS.some((p) => path === p || path.startsWith(p + '/'));
	}

	onMount(() => {
		theme.init();
		void bootstrap();
	});

	async function bootstrap(): Promise<void> {
		try {
			const info = await session.loadSession();
			if (info === null) {
				// 401: the API client already redirects to /login for non-public paths.
				return;
			}
			// Land on the first member tenant when hitting the bare root.
			if (page.url.pathname === '/' && info.tenants[0]) {
				await goto('/' + info.tenants[0].id + '/');
			}
		} catch {
			// Network/parse failure — render anyway so public pages still work.
		} finally {
			ready = true;
		}
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Tallyo</title>
</svelte:head>

<div class="min-h-screen bg-gray-50 text-gray-900">
	{#if ready}
		{@render children()}
	{:else}
		<main class="mx-auto w-full max-w-4xl flex-1 px-4 py-8">
			<p class="text-sm text-gray-500">Loading…</p>
		</main>
	{/if}
</div>
