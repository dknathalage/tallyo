<script lang="ts">
	import { page } from '$app/state';
	import { base } from '$app/paths';

	const docsHome = `${base}/docs`;

	const sections = [
		{
			title: 'Introduction',
			items: [
				{ href: docsHome, label: 'Overview' },
				{ href: `${base}/docs/getting-started`, label: 'Getting Started' },
				{ href: `${base}/docs/features`, label: 'Features' },
				{ href: `${base}/docs/architecture`, label: 'Architecture' }
			]
		},
		{
			title: 'Guides',
			items: [
				{ href: `${base}/docs/guides/invoices`, label: 'Invoices' },
				{ href: `${base}/docs/guides/estimates`, label: 'Estimates' },
				{ href: `${base}/docs/guides/clients`, label: 'Clients' },
				{ href: `${base}/docs/guides/catalog`, label: 'Catalog' },
				{ href: `${base}/docs/guides/import-export`, label: 'Import & Export' },
				{ href: `${base}/docs/guides/pdf-generation`, label: 'PDF Generation' },
				{ href: `${base}/docs/guides/settings`, label: 'Settings' }
			]
		}
	];

	function isActive(href: string): boolean {
		const path = page.url.pathname;
		if (href === docsHome) return path === docsHome || path === `${docsHome}/`;
		return path.startsWith(href);
	}
</script>

<nav class="w-64 shrink-0" aria-label="Documentation navigation">
	<div class="space-y-6">
		{#each sections as section (section.title)}
			<div>
				<h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{section.title}</h3>
				<ul class="space-y-1">
					{#each section.items as item (item.href)}
						<li>
							<a
								href={item.href}
								class="block rounded-md px-3 py-2 text-sm font-medium transition-colors {isActive(item.href)
									? 'bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300'
									: 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white'}"
							>
								{item.label}
							</a>
						</li>
					{/each}
				</ul>
			</div>
		{/each}
	</div>
</nav>
