<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { apiGet, apiPost } from '$lib/api/client';
	import type { SetupStatus, User } from '$lib/api/types';

	let { children } = $props();

	let ready = $state(false);
	let user = $state<User | null>(null);

	const PUBLIC_PATHS = ['/login', '/setup', '/accept-invite'];

	function isPublic(path: string): boolean {
		return PUBLIC_PATHS.some((p) => path === p || path.startsWith(p + '/'));
	}

	onMount(() => {
		void bootstrap();
	});

	async function bootstrap(): Promise<void> {
		try {
			const status = await apiGet<SetupStatus>('/api/setup/status');
			if (status !== null && status.ownerExists === false) {
				if (page.url.pathname !== '/setup') {
					await goto('/setup');
				}
				ready = true;
				return;
			}

			const me = await apiGet<User>('/api/auth/me');
			user = me;
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
		try {
			await apiPost('/api/auth/logout');
		} catch {
			// Ignore — clear local state regardless.
		}
		user = null;
		await goto('/login');
	}

	const showNav = $derived(user !== null && !isPublic(page.url.pathname));
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Tallyo</title>
</svelte:head>

<div class="min-h-screen bg-gray-50 text-gray-900">
	{#if showNav}
		<header class="border-b border-gray-200 bg-white">
			<nav class="mx-auto flex max-w-4xl items-center justify-between px-4 py-3">
				<a href="/" class="text-lg font-semibold">Tallyo</a>
				<div class="flex items-center gap-4 text-sm">
					<a href="/rate-tiers" class="text-gray-600 hover:text-gray-900">Rate Tiers</a>
						<a href="/tax-rates" class="text-gray-600 hover:text-gray-900">Tax Rates</a>
						<a href="/payers" class="text-gray-600 hover:text-gray-900">Payers</a>
						<a href="/clients" class="text-gray-600 hover:text-gray-900">Clients</a>
						<a href="/catalog" class="text-gray-600 hover:text-gray-900">Catalog</a>
						<a href="/settings" class="text-gray-600 hover:text-gray-900">Settings</a>
					<button
						type="button"
						onclick={logout}
						class="text-gray-600 hover:text-gray-900"
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
