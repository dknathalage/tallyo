<script lang="ts">
	import { getActivity } from '$lib/stores/activity.svelte.js';

	const activity = $derived(getActivity());
	const pct = $derived.by(() => {
		const p = activity.progress;
		if (!p || p.total <= 0) return null;
		return Math.min(100, Math.round((p.bytes / p.total) * 100));
	});

	function fmtBytes(n: number): string {
		if (n < 1024) return `${n} B`;
		if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
		if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
		return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
	}
</script>

<div class="border-t border-gray-200 px-3 py-2 text-xs dark:border-gray-700">
	{#if activity.id}
		<div class="flex items-center gap-2 text-gray-700 dark:text-gray-300">
			<span class="inline-block h-2 w-2 shrink-0 animate-pulse rounded-full bg-blue-500"></span>
			<span class="truncate">
				{activity.label}{#if activity.stage} · {activity.stage}{/if}
			</span>
		</div>
		{#if pct !== null && activity.progress}
			<div class="mt-1 flex items-center gap-2">
				<div class="h-1 flex-1 overflow-hidden rounded bg-gray-200 dark:bg-gray-700">
					<div class="h-full bg-blue-500 transition-all" style="width: {pct}%"></div>
				</div>
				<span class="shrink-0 tabular-nums text-gray-500 dark:text-gray-400">
					{fmtBytes(activity.progress.bytes)} / {fmtBytes(activity.progress.total)}
				</span>
			</div>
		{/if}
	{:else}
		<div class="flex items-center gap-2 text-gray-400 dark:text-gray-500">
			<span class="inline-block h-2 w-2 shrink-0 rounded-full bg-gray-300 dark:bg-gray-600"></span>
			<span>Idle</span>
		</div>
	{/if}
</div>
