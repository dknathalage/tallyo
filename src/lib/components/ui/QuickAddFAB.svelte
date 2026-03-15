<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { fade, scale } from 'svelte/transition';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let open = $state(false);

	function toggle() {
		open = !open;
	}

	function navigate(path: string) {
		open = false;
		goto(path);
	}

	const items = [
		{
			labelKey: 'quickAdd.newInvoice',
			path: `${base}/console/invoices/new`,
			icon: 'M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m3.75 9v6m3-3H9m1.5-12H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z'
		},
		{
			labelKey: 'quickAdd.newEstimate',
			path: `${base}/console/estimates/new`,
			icon: 'M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 002.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 00-1.123-.08m-5.801 0c-.065.21-.1.433-.1.664 0 .414.336.75.75.75h4.5a.75.75 0 00.75-.75 2.25 2.25 0 00-.1-.664m-5.8 0A2.251 2.251 0 0113.5 2.25H15c1.012 0 1.867.668 2.15 1.586m-5.8 0c-.376.023-.75.05-1.124.08C9.095 4.01 8.25 4.973 8.25 6.108V8.25m0 0H4.875c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125V9.375c0-.621-.504-1.125-1.125-1.125H8.25z'
		},
		{
			labelKey: 'quickAdd.newClient',
			path: `${base}/console/clients/new`,
			icon: 'M19 7.5v3m0 0v3m0-3h3m-3 0h-3m-2.25-4.125a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zM4 19.235v-.11a6.375 6.375 0 0112.75 0v.109A12.318 12.318 0 0110.374 21c-2.331 0-4.512-.645-6.374-1.766z'
		}
	];
</script>

{#if open}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-40"
		onclick={() => (open = false)}
		onkeydown={(e) => { if (e.key === 'Escape') open = false; }}
		role="presentation"
		transition:fade={{ duration: 100 }}
	></div>
{/if}

<div class="fixed bottom-6 right-6 z-50 flex flex-col items-end gap-2">
	{#if open}
		<div class="flex flex-col gap-2" transition:scale={{ duration: 150, start: 0.9 }}>
			{#each items as item}
				<button
					onclick={() => navigate(item.path)}
					class="flex cursor-pointer items-center gap-3 rounded-full bg-white py-2 pl-4 pr-3 text-sm font-medium text-gray-700 shadow-lg ring-1 ring-gray-200 transition-all hover:bg-gray-50 hover:shadow-xl dark:bg-gray-800 dark:text-gray-200 dark:ring-gray-700 dark:hover:bg-gray-700"
				>
					<span class="whitespace-nowrap">{i18n.t(item.labelKey)}</span>
					<svg class="h-5 w-5 shrink-0" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
						<path stroke-linecap="round" stroke-linejoin="round" d={item.icon} />
					</svg>
				</button>
			{/each}
		</div>
	{/if}

	<button
		onclick={toggle}
		class="flex h-14 w-14 cursor-pointer items-center justify-center rounded-full bg-primary-600 text-white shadow-lg transition-all hover:bg-primary-700 hover:shadow-xl focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900"
		class:rotate-45={open}
		aria-label={i18n.t('quickAdd.label')}
	>
		<svg class="h-6 w-6 transition-transform duration-200" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor">
			<path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
		</svg>
	</button>
</div>
