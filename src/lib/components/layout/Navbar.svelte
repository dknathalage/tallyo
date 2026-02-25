<script lang="ts">
	import { page } from '$app/state';
	import { base } from '$app/paths';
	import { db, close } from '$lib/db/connection.svelte';

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

<nav class="border-b border-gray-200 bg-white">
	<div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
		<div class="flex h-16 items-center justify-between">
			<!-- Left: Logo + Nav Links -->
			<div class="flex items-center gap-8">
				<a href="{base}/" class="flex items-center gap-2">
					<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary-600 text-sm font-bold text-white">
						IM
					</div>
					<span class="text-lg font-semibold text-gray-900">Invoice Manager</span>
				</a>

				<!-- Desktop nav -->
				<div class="hidden items-center gap-1 sm:flex">
					{#each navLinks as link}
						<a
							href={link.href}
							class="rounded-md px-3 py-2 text-sm font-medium transition-colors {isActive(link.href)
								? 'bg-primary-50 text-primary-700'
								: 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'}"
						>
							{link.label}
						</a>
					{/each}
				</div>
			</div>

			<!-- Right: File name + Close -->
			<div class="hidden items-center gap-4 sm:flex">
				<span class="text-sm text-gray-500">{db.fileName}</span>
				<button
					onclick={close}
					class="cursor-pointer rounded-md px-3 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
				>
					Close
				</button>
			</div>

			<!-- Mobile hamburger -->
			<button
				onclick={() => (mobileMenuOpen = !mobileMenuOpen)}
				class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-gray-600 hover:bg-gray-100 hover:text-gray-900 sm:hidden"
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

	<!-- Mobile menu -->
	{#if mobileMenuOpen}
		<div class="border-t border-gray-200 sm:hidden">
			<div class="space-y-1 px-4 py-3">
				{#each navLinks as link}
					<a
						href={link.href}
						onclick={() => (mobileMenuOpen = false)}
						class="block rounded-md px-3 py-2 text-sm font-medium {isActive(link.href)
							? 'bg-primary-50 text-primary-700'
							: 'text-gray-600 hover:bg-gray-50 hover:text-gray-900'}"
					>
						{link.label}
					</a>
				{/each}
			</div>
			<div class="border-t border-gray-200 px-4 py-3">
				<p class="text-sm text-gray-500">{db.fileName}</p>
				<button
					onclick={() => { mobileMenuOpen = false; close(); }}
					class="mt-2 cursor-pointer text-sm font-medium text-gray-600 hover:text-gray-900"
				>
					Close Database
				</button>
			</div>
		</div>
	{/if}
</nav>
