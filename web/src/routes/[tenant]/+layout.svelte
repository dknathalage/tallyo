<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { session } from '$lib/stores/session.svelte';
	import { features } from '$lib/stores/features.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { openEvents, closeEvents } from '$lib/realtime/events';
	import { t } from '$lib/nav';

	let { children } = $props();

	const tenant = $derived(page.params.tenant);

	// Path without the /{tenant} prefix — drives active-tab matching against the
	// unprefixed NAV_GROUPS hrefs below.
	const subPath = $derived.by(() => {
		const p = page.url.pathname;
		const prefix = '/' + tenant;
		if (p === prefix) return '/';
		if (p.startsWith(prefix + '/')) {
			const rest = p.slice(prefix.length);
			return rest === '' ? '/' : rest;
		}
		return p;
	});

	onMount(() => {
		// The root layout normally loads the session first, but a deep-link straight
		// into a tenant route can mount this layout before that completes.
		if (session.tenants.length === 0) {
			void session.loadSession();
		}
	});

	onDestroy(() => closeEvents());

	// Wire the active tenant whenever the route's tenant changes. We track the
	// last-wired uuid in a plain (non-reactive) variable so re-running the effect
	// for unrelated reactive reads (e.g. session.tenants arriving) does NOT redo
	// the loadMe/openEvents work or trigger a loop.
	let wired: string | null = null;

	$effect(() => {
		const uuid = tenant;
		if (!uuid) return; // always present on a [tenant] route, but the type allows undefined
		const known = session.tenants;

		// Membership check: only once tenants are known. Redirect a non-member away.
		if (known.length > 0) {
			const member = known.some((x) => x.id === uuid);
			if (!member) {
				untrack(() => {
					const fallback = known[0]?.id;
					void goto(fallback ? '/' + fallback + '/' : '/login');
				});
				return;
			}
		}

		if (uuid === wired) return;
		wired = uuid;

		untrack(() => {
			closeEvents();
			void (async () => {
				await session.loadMe();
				await features.load();
				openEvents();
			})();
		});
	});

	import type { Icon as IconType } from '@lucide/svelte';
	import LayoutGrid from '@lucide/svelte/icons/layout-grid';
	import FileText from '@lucide/svelte/icons/file-text';
	import Users from '@lucide/svelte/icons/users';
	import BookOpen from '@lucide/svelte/icons/book-open';
	import Settings from '@lucide/svelte/icons/settings';
	import Sun from '@lucide/svelte/icons/sun';
	import Moon from '@lucide/svelte/icons/moon';

	type NavChild = { href: string; label: string };
	type NavGroup = { label: string; icon: typeof IconType; children: NavChild[] };

	// Sidebar shows one entry per group (links to the first child); each page renders
	// the group's sub-tabs. Hrefs are UNPREFIXED here and matched against subPath;
	// rendered links go through t() to add the active tenant.
	const NAV_GROUPS: NavGroup[] = [
		{
			label: 'Shifts',
			icon: LayoutGrid,
			children: [{ href: '/', label: 'Shifts' }]
		},
		{
			label: 'Clients',
			icon: Users,
			children: [
				{ href: '/clients', label: 'Clients' },
				{ href: '/payers', label: 'Payers' }
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

	// Match length of href against path (-1 = no match). Used to pick the most
	// specific sibling when routes nest, e.g. /settings vs /settings/users.
	function matchLen(href: string, path: string): number {
		// The Shifts home route only matches exactly — otherwise it would match
		// every path (all paths start with '/').
		if (href === '/') return path === '/' ? href.length : -1;
		if (path === href || path.startsWith(href + '/')) return href.length;
		return -1;
	}

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

	const currentGroup = $derived(NAV_GROUPS.find((g) => activeChild(g, subPath) !== null) ?? null);
	const currentChild = $derived(currentGroup ? activeChild(currentGroup, subPath) : null);
	const showTabs = $derived(currentGroup !== null && currentGroup.children.length > 1);

	function switchTenant(uuid: string): void {
		if (uuid && uuid !== tenant) void goto('/' + uuid + '/');
	}
</script>

<div class="md:flex">
	<header
		class="border-b border-gray-200 bg-white md:sticky md:top-0 md:h-screen md:w-56 md:shrink-0 md:overflow-y-auto md:border-b-0 md:border-r"
	>
		<nav
			class="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 md:h-full md:flex-col md:flex-nowrap md:items-stretch md:gap-y-1 md:px-3 md:py-4"
		>
			<a href={t('/')} class="shrink-0 text-lg font-semibold md:mb-2 md:px-2">Tallyo</a>

			{#if session.tenants.length > 1}
				<label class="sr-only" for="tenant-switcher">Organisation</label>
				<select
					id="tenant-switcher"
					value={tenant}
					onchange={(e) => switchTenant((e.currentTarget as HTMLSelectElement).value)}
					class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm md:mb-2"
				>
					{#each session.tenants as ten (ten.id)}
						<option value={ten.id}>{ten.tenantName}</option>
					{/each}
				</select>
			{/if}

			<div
				class="flex flex-1 flex-wrap items-center justify-end gap-x-3 gap-y-2 text-sm md:w-full md:flex-col md:flex-nowrap md:items-stretch md:justify-start md:gap-y-1"
			>
				{#each NAV_GROUPS as group (group.label)}
					{@const active = activeChild(group, subPath) !== null}
					<a
						href={t(group.children[0].href)}
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

	<main class="mx-auto w-full max-w-4xl flex-1 px-4 py-8 md:min-w-0">
		{#if showTabs && currentGroup}
			<nav class="mb-6 flex flex-wrap gap-x-1 border-b border-gray-200">
				{#each currentGroup.children as child (child.href)}
					{@const active = child.href === currentChild?.href}
					<a
						href={t(child.href)}
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
	</main>
</div>
