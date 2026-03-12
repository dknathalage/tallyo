<script lang="ts">
	import FileGate from '$lib/components/layout/FileGate.svelte';
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import KeyboardShortcuts from '$lib/components/layout/KeyboardShortcuts.svelte';
	import ToastContainer from '$lib/components/layout/ToastContainer.svelte';

	let { children } = $props();
</script>

<FileGate>
	<AppShell>
		{@render children()}
	</AppShell>
</FileGate>

<KeyboardShortcuts />
<ToastContainer />
