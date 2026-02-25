<script lang="ts">
	import '../app.css';
	import FileGate from '$lib/components/layout/FileGate.svelte';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import ReloadPrompt from '$lib/components/pwa/ReloadPrompt.svelte';
	import { pwaInfo } from 'virtual:pwa-info';

	let { children } = $props();

	const webManifestLink = pwaInfo ? pwaInfo.webManifest.linkTag : '';
</script>

<svelte:head>
	{@html webManifestLink}
</svelte:head>

<FileGate>
	<AppShell>
		{@render children()}
	</AppShell>
</FileGate>

<ReloadPrompt />
