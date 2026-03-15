<script lang="ts">
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import KeyboardShortcuts from '$lib/components/layout/KeyboardShortcuts.svelte';
	import ToastContainer from '$lib/components/layout/ToastContainer.svelte';

	let { children } = $props();
</script>

<AppShell>
	{@render children()}
</AppShell>

<KeyboardShortcuts />
<ToastContainer />
