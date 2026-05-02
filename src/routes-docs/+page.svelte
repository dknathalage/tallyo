<script lang="ts">
	import { base } from '$app/paths';
	import { theme } from '$lib/stores/theme.svelte';

	const RELEASES = 'https://github.com/dknathalage/tallyo/releases/latest';

	const features = [
		{ title: 'Local-First', details: 'Your data lives on your device in SQLite. No servers, no telemetry, no subscriptions.' },
		{ title: 'Invoice Management', details: 'Create, edit, and track invoices with line items, client linking, and automatic numbering.' },
		{ title: 'PDF Export', details: 'Generate professional PDF invoices with your business branding, ready to send.' },
		{ title: 'Import & Export', details: 'Bulk import and export data via CSV and Excel files with a guided wizard.' },
		{ title: 'Estimates & Quotes', details: 'Send prospective pricing to clients and convert accepted estimates into invoices with one click.' },
		{ title: 'Multi-Currency', details: 'Create invoices and estimates in 20+ currencies with locale-aware formatting.' },
		{ title: 'Dark Mode', details: 'Switch between light, dark, and system themes for comfortable viewing any time of day.' },
		{ title: 'Internationalization', details: 'Locale-aware date and number formatting with a translation-ready architecture.' }
	];
</script>

<svelte:head>
	<title>Tallyo — Local-First Invoice Management</title>
</svelte:head>

<div class="min-h-screen bg-white dark:bg-gray-950">
	<nav class="border-b border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
		<div class="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
			<div class="flex items-center gap-2">
				<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary-600 text-sm font-bold text-white">T</div>
				<span class="text-lg font-semibold text-gray-900 dark:text-white">Tallyo</span>
			</div>
			<div class="flex items-center gap-4">
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
				<a href={`${base}/docs`} class="rounded-md px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-white">Docs</a>
				<a href={RELEASES} class="rounded-md bg-primary-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-700">Download</a>
			</div>
		</div>
	</nav>

	<div class="py-16 text-center">
		<div class="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-primary-600 text-3xl font-bold text-white">T</div>
		<h1 class="text-4xl font-bold tracking-tight text-gray-900 dark:text-white sm:text-5xl">Tallyo</h1>
		<p class="mt-2 text-xl font-medium text-primary-600 dark:text-primary-400">Local-First Invoice Management</p>
		<p class="mx-auto mt-4 max-w-2xl text-lg text-gray-600 dark:text-gray-400">Your data stays on your device. No servers, no subscriptions.</p>
		<div class="mt-8 flex items-center justify-center gap-4">
			<a href={RELEASES} class="rounded-lg bg-primary-600 px-6 py-3 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-primary-700">Download for your platform</a>
			<a href={`${base}/docs/getting-started`} class="rounded-lg border border-gray-300 bg-white px-6 py-3 text-sm font-semibold text-gray-700 shadow-sm transition-colors hover:bg-gray-50 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700">Get Started</a>
		</div>
	</div>

	<div class="mx-auto max-w-7xl px-4 pb-16 sm:px-6 lg:px-8">
		<div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
			{#each features as feature (feature.title)}
				<div class="rounded-xl border border-gray-200 p-6 dark:border-gray-700">
					<h3 class="font-semibold text-gray-900 dark:text-white">{feature.title}</h3>
					<p class="mt-2 text-sm text-gray-600 dark:text-gray-400">{feature.details}</p>
				</div>
			{/each}
		</div>
	</div>
</div>
