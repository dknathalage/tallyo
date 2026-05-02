<script lang="ts">
	import '../app.css';
	import { theme } from '$lib/stores/theme.svelte';
	import { onMount, type Snippet } from 'svelte';

	const { children }: { children: Snippet } = $props();

	onMount(() => {
		theme.init();
	});
</script>

{@render children()}
