<script lang="ts">
	import { getToasts, removeToast } from '$lib/stores/toast.svelte.js';

	const toastTypeClasses: Record<string, string> = {
		success: 'bg-green-500 text-white',
		error: 'bg-red-500 text-white',
		warning: 'bg-yellow-400 text-gray-900',
		info: 'bg-blue-500 text-white'
	};

	const toastIcons: Record<string, string> = {
		success: '✓',
		error: '✕',
		warning: '⚠',
		info: 'ℹ'
	};
</script>

<div class="fixed top-4 right-4 z-50 flex flex-col gap-2 max-w-sm w-full pointer-events-none">
	{#each getToasts() as toast (toast.id)}
		<div
			class="flex items-start gap-3 px-4 py-3 rounded-lg shadow-lg pointer-events-auto
				{toastTypeClasses[toast.type] ?? toastTypeClasses['info']}"
			role="alert"
		>
			<span class="font-bold text-lg leading-none mt-0.5">{toastIcons[toast.type]}</span>
			<p class="flex-1 text-sm font-medium">{toast.message}</p>
			<button
				onclick={() => removeToast(toast.id)}
				class="ml-2 opacity-70 hover:opacity-100 transition-opacity text-lg leading-none"
				aria-label="Dismiss"
			>×</button>
		</div>
	{/each}
</div>
