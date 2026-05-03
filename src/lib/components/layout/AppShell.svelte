<script lang="ts">
	import type { Snippet } from 'svelte';
	import Sidebar from './Sidebar.svelte';
	import ChatPanel from '$lib/components/ai/ChatPanel.svelte';
	import { getChat, toggleOpen as toggleChat } from '$lib/stores/ai-chat.svelte.js';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	const { children }: { children: Snippet } = $props();

	let sidebarOpen = $state(false);
	const chat = $derived(getChat());
</script>

<div class="min-h-screen bg-gray-50 dark:bg-gray-950">
	<a href="#main-content" class="sr-only focus:fixed focus:left-2 focus:top-2 focus:z-[100] focus:rounded-md focus:bg-primary-600 focus:px-4 focus:py-2 focus:text-white focus:shadow-lg">{i18n.t('a11y.skipToContent')}</a>

	<Sidebar bind:open={sidebarOpen} />

	<!-- Top bar -->
	<div class="sticky top-0 z-30 flex h-12 items-center justify-between gap-3 border-b border-gray-200 bg-white px-4 dark:border-gray-700 dark:bg-gray-800 lg:ml-64 {chat.open ? 'lg:mr-96' : ''}">
		<div class="flex items-center gap-3 lg:invisible">
			<button
				onclick={() => (sidebarOpen = true)}
				class="cursor-pointer rounded-md p-2 text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-gray-700 dark:hover:text-white lg:hidden"
				aria-label={i18n.t('a11y.toggleMenu')}
			>
				<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
				</svg>
			</button>
			<span class="text-base font-semibold text-gray-900 dark:text-white lg:hidden">{i18n.t('nav.appName')}</span>
		</div>
		<button
			type="button"
			onclick={toggleChat}
			class="flex items-center gap-2 rounded-md border border-gray-200 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-200 dark:hover:bg-gray-700 {chat.open ? 'border-blue-500 text-blue-700 dark:border-blue-400 dark:text-blue-300' : ''}"
			aria-label="Toggle AI chat"
			title="Toggle AI chat (⌘K)"
		>
			<svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
				<path stroke-linecap="round" stroke-linejoin="round" d="M8.625 12a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0H8.25m4.125 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0H12m4.125 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0h-.375M21 12c0 4.556-4.03 8.25-9 8.25a9.764 9.764 0 01-2.555-.337A5.972 5.972 0 015.41 20.97a5.969 5.969 0 01-.474-.065 4.48 4.48 0 00.978-2.025c.09-.457-.133-.901-.467-1.226C3.93 16.178 3 14.189 3 12c0-4.556 4.03-8.25 9-8.25s9 3.694 9 8.25z" />
			</svg>
			<span>AI</span>
			<span class="rounded bg-gray-100 px-1 py-0.5 font-mono text-[10px] text-gray-500 dark:bg-gray-700 dark:text-gray-400">⌘K</span>
		</button>
	</div>

	<main id="main-content" tabindex="-1" class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:ml-64 lg:px-8 {chat.open ? 'lg:mr-96' : ''}">
		{@render children()}
	</main>

	<ChatPanel />
</div>
