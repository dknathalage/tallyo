<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { session } from '$lib/stores/session.svelte';

	let { children } = $props();

	let ready = $state(false);

	const PUBLIC_PATHS = ['/login', '/signup', '/accept-invite'];

	function isPublic(path: string): boolean {
		return PUBLIC_PATHS.some((p) => path === p || path.startsWith(p + '/'));
	}

	onMount(() => {
		void bootstrap();
	});

	async function bootstrap(): Promise<void> {
		try {
			const me = await session.refresh();
			if (me === null && !isPublic(page.url.pathname)) {
				await goto('/login');
			}
		} catch {
			// Network/parse failure — render anyway so public pages still work.
		} finally {
			ready = true;
		}
	}

	async function logout(): Promise<void> {
		await session.logout();
		await goto('/login');
	}

	const showNav = $derived(session.user !== null && !isPublic(page.url.pathname));
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Tallyo</title>
</svelte:head>

<div class="min-h-screen bg-gray-50 text-gray-900">
	{#if showNav}
		<header class="border-b border-gray-200 bg-white">
			<nav class="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
				<a href="/" class="shrink-0 text-lg font-semibold">Tallyo</a>
				<div class="flex flex-1 flex-wrap items-center gap-x-4 gap-y-2 text-sm">
					<a href="/invoices" class="whitespace-nowrap text-gray-600 hover:text-gray-900">Invoices</a>
					<a href="/estimates" class="whitespace-nowrap text-gray-600 hover:text-gray-900">Estimates</a>
					<a href="/recurring" class="whitespace-nowrap text-gray-600 hover:text-gray-900">Recurring</a>
					<a href="/participants" class="whitespace-nowrap text-gray-600 hover:text-gray-900"
						>Participants</a
					>
					<a href="/plan-managers" class="whitespace-nowrap text-gray-600 hover:text-gray-900"
						>Plan managers</a
					>
					<a href="/custom-items" class="whitespace-nowrap text-gray-600 hover:text-gray-900"
						>Custom items</a
					>
					<a href="/support-catalog" class="whitespace-nowrap text-gray-600 hover:text-gray-900"
						>Support catalogue</a
					>
					<a href="/tax-rates" class="whitespace-nowrap text-gray-600 hover:text-gray-900">Tax rates</a>
					<a href="/settings" class="whitespace-nowrap text-gray-600 hover:text-gray-900">Settings</a>
					<button
						type="button"
						onclick={logout}
						class="ml-auto whitespace-nowrap text-gray-600 hover:text-gray-900"
					>
						Logout
					</button>
				</div>
			</nav>
		</header>
	{/if}

	<main class="mx-auto max-w-4xl px-4 py-8">
		{#if ready}
			{@render children()}
		{:else}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
	</main>
</div>
