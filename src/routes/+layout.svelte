<script lang="ts">
	import '../app.css';
	import ReloadPrompt from '$lib/components/pwa/ReloadPrompt.svelte';
	import LiveAnnouncer from '$lib/components/shared/LiveAnnouncer.svelte';
	import { pwaInfo } from 'virtual:pwa-info';
	import { theme } from '$lib/stores/theme.svelte';
	import { i18n } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	let { children } = $props();

	const webManifestLink = pwaInfo ? pwaInfo.webManifest.linkTag : '';

	onMount(() => {
		theme.init();
		i18n.init();
	});
</script>

<svelte:head>
	{@html webManifestLink}
</svelte:head>

{@render children()}

<LiveAnnouncer />
<ReloadPrompt />
