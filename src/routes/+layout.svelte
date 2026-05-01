<script lang="ts">
	import '../app.css';
	import LiveAnnouncer from '$lib/components/shared/LiveAnnouncer.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { i18n } from '$lib/stores/i18n.svelte';
	import { onMount, type Snippet } from 'svelte';

	const { children }: { children: Snippet } = $props();

	onMount(() => {
		theme.init();
		i18n.init();
	});
</script>

{@render children()}

<LiveAnnouncer />
