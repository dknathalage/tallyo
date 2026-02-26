<script lang="ts">
	import { useRegisterSW } from 'virtual:pwa-register/svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	const intervalMS = 60 * 60 * 1000; // check for updates every hour

	const {
		needRefresh,
		offlineReady,
		updateServiceWorker
	} = useRegisterSW({
		onRegisteredSW(swUrl, registration) {
			if (registration) {
				setInterval(() => {
					registration.update();
				}, intervalMS);
			}
		},
		onRegisterError(error) {
			console.error('SW registration error:', error);
		}
	});

	function close() {
		offlineReady.set(false);
		needRefresh.set(false);
	}

	function handleUpdate() {
		updateServiceWorker(true);
	}

	$: showPrompt = $offlineReady || $needRefresh;
</script>

{#if showPrompt}
	<div
		class="fixed bottom-4 right-4 z-50 flex items-center gap-3 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-3 shadow-lg"
		role="alert"
	>
		{#if $offlineReady}
			<p class="text-sm text-gray-700 dark:text-gray-300">{i18n.t('pwa.offlineReady')}</p>
		{:else}
			<p class="text-sm text-gray-700 dark:text-gray-300">{i18n.t('pwa.newVersionAvailable')}</p>
		{/if}

		<div class="flex gap-2">
			{#if $needRefresh}
				<button
					onclick={handleUpdate}
					class="rounded bg-blue-600 px-3 py-1 text-sm font-medium text-white hover:bg-blue-700"
				>
					{i18n.t('pwa.reload')}
				</button>
			{/if}
			<button
				onclick={close}
				class="rounded border border-gray-300 dark:border-gray-600 px-3 py-1 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700"
			>
				{i18n.t('pwa.dismiss')}
			</button>
		</div>
	</div>
{/if}
