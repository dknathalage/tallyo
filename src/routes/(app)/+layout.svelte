<script lang="ts">
	import FileGate from '$lib/components/layout/FileGate.svelte';
	import AppShell from '$lib/components/layout/AppShell.svelte';

	let { children } = $props();
</script>

<FileGate>
	<AppShell>
		{@render children()}
	</AppShell>
</FileGate>
