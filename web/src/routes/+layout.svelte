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

	const showNav = $derived(session.user !== null && !isPublic(page.url.pathname));

	// Match length of href against path (-1 = no match). Used to pick the most
	// specific sibling when routes nest, e.g. /settings vs /settings/users.
	function matchLen(href: string, path: string): number {
		// The Shifts home route only matches exactly — otherwise it would match
		// every path (all paths start with '/').
		if (href === '/') return path === '/' ? href.length : -1;
		if (path === href || path.startsWith(href + '/')) return href.length;
		return -1;
	}

	import type { Icon as IconType } from '@lucide/svelte';
	import LayoutGrid from '@lucide/svelte/icons/layout-grid';
	import CalendarDays from '@lucide/svelte/icons/calendar-days';
	import FileText from '@lucide/svelte/icons/file-text';
	import Users from '@lucide/svelte/icons/users';
	import BookOpen from '@lucide/svelte/icons/book-open';
	import Settings from '@lucide/svelte/icons/settings';
	import Sun from '@lucide/svelte/icons/sun';
	import Moon from '@lucide/svelte/icons/moon';

	type NavChild = { href: string; label: string };
	type NavGroup = { label: string; icon: typeof IconType; children: NavChild[] };

	// Sidebar shows one entry per group (links to the first child); each page renders
	// the group's sub-tabs. Routes are unchanged — this is purely a visual grouping.
	const NAV_GROUPS: NavGroup[] = [
		{
			label: 'Shifts',
			icon: LayoutGrid,
			children: [{ href: '/', label: 'Shifts' }]
		},
//		{
//			label: 'Calendar',
//			icon: CalendarDays,
//			children: [{ href: '/calendar', label: 'Calendar' }]
//		},
		{
			label: 'Participants',
			icon: Users,
			children: [
				{ href: '/participants', label: 'Participants' },
				{ href: '/plan-managers', label: 'Plan managers' }
			]
		},
		{
			label: 'Invoices',
			icon: FileText,
			children: [
				{ href: '/invoices', label: 'Invoices' },
				{ href: '/estimates', label: 'Estimates' },
				{ href: '/recurring', label: 'Recurring' }
			]
		},
		{
			label: 'Catalog',
			icon: BookOpen,
			children: [
				{ href: '/custom-items', label: 'Custom items' },
				{ href: '/support-catalog', label: 'Support catalogue' },
				{ href: '/tax-rates', label: 'Tax rates' }
			]
		},
		{
			label: 'Settings',
			icon: Settings,
			children: [
				{ href: '/settings', label: 'Business profile' },
				{ href: '/settings/users', label: 'Users' },
				{ href: '/settings/account', label: 'Account' }
			]
		}
	];

	// The child whose href best (longest) matches the path — the active tab.
	function activeChild(group: NavGroup, path: string): NavChild | null {
		let best: NavChild | null = null;
		let bestLen = -1;
		for (const c of group.children) {
			const l = matchLen(c.href, path);
			if (l > bestLen) {
				bestLen = l;
				best = c;
			}
		}
		return best;
	}

	// The group owning the current route — drives the sub-tab row.
	const currentGroup = $derived(
		NAV_GROUPS.find((g) => activeChild(g, page.url.pathname) !== null) ?? null
	);
	const currentChild = $derived(
		currentGroup ? activeChild(currentGroup, page.url.pathname) : null
	);
	const showTabs = $derived(showNav && currentGroup !== null && currentGroup.children.length > 1);
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Tallyo</title>
</svelte:head>

<div class="min-h-screen bg-gray-50 text-gray-900 md:flex">
	{#if showNav}
		<header
			class="border-b border-gray-200 bg-white md:sticky md:top-0 md:h-screen md:w-56 md:shrink-0 md:overflow-y-auto md:border-b-0 md:border-r"
		>
			<nav
				class="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 md:h-full md:flex-col md:flex-nowrap md:items-stretch md:gap-y-1 md:px-3 md:py-4"
			>
				<a href="/" class="shrink-0 text-lg font-semibold md:mb-2 md:px-2">Tallyo</a>
				<div
					class="flex flex-1 flex-wrap items-center justify-end gap-x-3 gap-y-2 text-sm md:w-full md:flex-col md:flex-nowrap md:items-stretch md:justify-start md:gap-y-1"
				>
					{#each NAV_GROUPS as group (group.label)}
						{@const active = activeChild(group, page.url.pathname) !== null}
						<a
							href={group.children[0].href}
							title={group.label}
							aria-label={group.label}
							aria-current={active ? 'page' : undefined}
							class="flex items-center gap-2 whitespace-nowrap md:rounded md:px-2 md:py-1.5 {active
								? 'text-gray-900 md:bg-gray-100 md:font-medium'
								: 'text-gray-600 hover:text-gray-900 md:hover:bg-gray-100'}"
						>
							<group.icon class="size-5 shrink-0" aria-hidden="true" />
							<span class="hidden md:inline">{group.label}</span>
						</a>
					{/each}
					<button
						type="button"
						onclick={() => theme.toggle()}
						title={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}
						aria-label={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}
						class="flex items-center gap-2 whitespace-nowrap text-gray-600 hover:text-gray-900 md:mt-auto md:rounded md:px-2 md:py-1.5 md:hover:bg-gray-100"
					>
						{#if theme.isDark}
							<Sun class="size-5 shrink-0" aria-hidden="true" />
						{:else}
							<Moon class="size-5 shrink-0" aria-hidden="true" />
						{/if}
						<span class="hidden md:inline">{theme.isDark ? 'Light mode' : 'Dark mode'}</span>
					</button>
				</div>
			</nav>
		</header>
	{/if}

	<main class="mx-auto w-full max-w-4xl flex-1 px-4 py-8 md:min-w-0">
		{#if ready}
			{#if showTabs && currentGroup}
				<nav class="mb-6 flex flex-wrap gap-x-1 border-b border-gray-200">
					{#each currentGroup.children as child (child.href)}
						{@const active = child.href === currentChild?.href}
						<a
							href={child.href}
							aria-current={active ? 'page' : undefined}
							class="-mb-px border-b-2 px-3 py-2 text-sm {active
								? 'border-gray-900 font-medium text-gray-900'
								: 'border-transparent text-gray-600 hover:text-gray-900'}"
						>
							{child.label}
						</a>
					{/each}
				</nav>
			{/if}
			{@render children()}
		{:else}
			<p class="text-sm text-gray-500">Loading…</p>
		{/if}
	</main>
</div>
