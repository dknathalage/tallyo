<script lang="ts">
	import { resolve } from '$app/paths';

	const features = [
		{ title: 'Fully Offline', details: 'Works entirely in your browser with no backend server. Data is stored locally using IndexedDB and SQL.js.' },
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
	<title>Tallyo — Documentation</title>
</svelte:head>

<div class="not-prose">
	<div class="py-16 text-center">
		<div class="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-primary-600 text-3xl font-bold text-white">
			T
		</div>
		<h1 class="text-4xl font-bold tracking-tight text-gray-900 dark:text-white sm:text-5xl">
			Tallyo
		</h1>
		<p class="mt-2 text-xl font-medium text-primary-600 dark:text-primary-400">
			Local-First Invoice Management
		</p>
		<p class="mx-auto mt-4 max-w-2xl text-lg text-gray-600 dark:text-gray-400">
			Your data stays on your device. No servers, no subscriptions.
		</p>
		<div class="mt-8 flex items-center justify-center gap-4">
			<a
				href={resolve('/(docs)/docs/getting-started')}
				class="rounded-lg bg-primary-600 px-6 py-3 text-sm font-semibold text-white shadow-sm hover:bg-primary-700 transition-colors"
			>
				Get Started
			</a>
			<a
				href={resolve('/console')}
				class="rounded-lg border border-gray-300 bg-white px-6 py-3 text-sm font-semibold text-gray-700 shadow-sm hover:bg-gray-50 transition-colors dark:border-gray-600 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700"
			>
				Open App
			</a>
		</div>
	</div>

	<div class="mt-8 grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
		{#each features as feature (feature.title)}
			<div class="rounded-xl border border-gray-200 p-6 dark:border-gray-700">
				<h3 class="font-semibold text-gray-900 dark:text-white">{feature.title}</h3>
				<p class="mt-2 text-sm text-gray-600 dark:text-gray-400">{feature.details}</p>
			</div>
		{/each}
	</div>
</div>
