<script lang="ts">
	import type { Snippet } from 'svelte';

	let { type = 'tip', children }: { type?: 'tip' | 'warning'; children: Snippet } = $props();

	const styles = {
		tip: {
			container: 'border-l-4 border-blue-500 bg-blue-50 dark:bg-blue-900/20 dark:border-blue-400',
			title: 'text-blue-800 dark:text-blue-300',
			label: 'Tip'
		},
		warning: {
			container: 'border-l-4 border-amber-500 bg-amber-50 dark:bg-amber-900/20 dark:border-amber-400',
			title: 'text-amber-800 dark:text-amber-300',
			label: 'Warning'
		}
	};

	const style = $derived(styles[type]);
</script>

<div class="{style.container} rounded-r-lg p-4">
	<p class="mb-1 text-sm font-bold {style.title}">{style.label}</p>
	<div class="text-sm text-gray-700 dark:text-gray-300">
		{@render children()}
	</div>
</div>
