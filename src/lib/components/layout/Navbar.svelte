<script lang="ts">
	import { page } from '$app/state';
	import { base } from '$app/paths';
	import { db, close } from '$lib/db/connection.svelte';
	import { theme } from '$lib/stores/theme.svelte';

	let mobileMenuOpen = $state(false);

	const navLinks = [
		{ href: `${base}/`, label: 'Dashboard' },
		{ href: `${base}/invoices`, label: 'Invoices' },
		{ href: `${base}/clients`, label: 'Clients' },
		{ href: `${base}/catalog`, label: 'Catalog' },
		{ href: `${base}/settings`, label: 'Settings' }
	];

	function isActive(href: string): boolean {
		const path = page.url.pathname;
		if (href === `${base}/`) return path === `${base}/` || path === base;
		return path.startsWith(href);
	}
</script>

<nav class="border-b border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
	<div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
		<div class="flex h-16 items-center justify-between">
			<!-- Left: Logo + Nav Links -->
			<div class="flex items-center gap-8">
				<a href="{base}/" class="flex items-center gap-2">
					<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary-600 text-sm font-bold text-white">
						IM
					</div>
					<span class="text-lg font-semibold text-gray-900 dark:text-white">Invoice Manager</span>
				</a>

				<!-- Desktop nav -->
				<div class="hidden items-center gap-1 sm:flex">
					{#each navLinks as link}
						<a
							href={link.href}
							class="rounded-md px-3 py-2 text-sm font-medium transition-colors {isActive(link.href)
								? 'bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300'
								: 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-white'}"
						>
							{link.label}
						</a>
					{/each}
				</div>
			</div>

			<!-- Right: Theme toggle + File name + Close -->
			<div class="hidden items-center gap-4 sm:flex">
				<button
					onclick={() => theme.toggle()}
					class="cursor-pointer rounded-md p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
					aria-label="Toggle dark mode"
				>
					{#if theme.isDark}
						<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z" />
						</svg>
					{:else}
						<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" d="M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z" />
						</svg>
					{/if}
				</button>
				<span class="text-sm text-gray-500 dark:text-gray-400">{db.fileName}</span>
				<button
					onclick={close}
					class="cursor-pointer rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-white"
				>
					Close
				</button>
			</div>

			<!-- Mobile hamburger -->
			<div class="flex items-center gap-2 sm:hidden">
				<button
					onclick={() => theme.toggle()}
					class="cursor-pointer rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
					aria-label="Toggle dark mode"
				>
					{#if theme.isDark}
						<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z" />
						</svg>
					{:else}
						<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" d="M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z" />
						</svg>
					{/if}
				</button>
				<button
					onclick={() => (mobileMenuOpen = !mobileMenuOpen)}
					class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-white"
					aria-label="Toggle menu"
				>
					<svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						{#if mobileMenuOpen}
							<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
						{:else}
							<path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
						{/if}
					</svg>
				</button>
			</div>
		</div>
	</div>

	<!-- Mobile menu -->
	{#if mobileMenuOpen}
		<div class="border-t border-gray-200 dark:border-gray-700 sm:hidden">
			<div class="space-y-1 px-4 py-3">
				{#each navLinks as link}
					<a
						href={link.href}
						onclick={() => (mobileMenuOpen = false)}
						class="block rounded-md px-3 py-2 text-sm font-medium {isActive(link.href)
							? 'bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300'
							: 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-white'}"
					>
						{link.label}
					</a>
				{/each}
			</div>
			<div class="border-t border-gray-200 px-4 py-3 dark:border-gray-700">
				<p class="text-sm text-gray-500 dark:text-gray-400">{db.fileName}</p>
				<button
					onclick={() => { mobileMenuOpen = false; close(); }}
					class="mt-2 cursor-pointer text-sm font-medium text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white"
				>
					Close Database
				</button>
			</div>
		</div>
	{/if}
</nav>
