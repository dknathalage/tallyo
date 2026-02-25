<script lang="ts">
	import { useRegisterSW } from 'virtual:pwa-register/svelte';

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
		class="fixed bottom-4 right-4 z-50 flex items-center gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 shadow-lg"
		role="alert"
	>
		{#if $offlineReady}
			<p class="text-sm text-gray-700">Ready to work offline</p>
		{:else}
			<p class="text-sm text-gray-700">New version available</p>
		{/if}

		<div class="flex gap-2">
			{#if $needRefresh}
				<button
					onclick={handleUpdate}
					class="rounded bg-blue-600 px-3 py-1 text-sm font-medium text-white hover:bg-blue-700"
				>
					Reload
				</button>
			{/if}
			<button
				onclick={close}
				class="rounded border border-gray-300 px-3 py-1 text-sm text-gray-600 hover:bg-gray-50"
			>
				Dismiss
			</button>
		</div>
	</div>
{/if}
