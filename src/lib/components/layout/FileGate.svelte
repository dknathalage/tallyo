<script lang="ts">
	import type { Snippet } from 'svelte';
	const { children }: { children: Snippet } = $props();
</script>

<!-- FileGate removed: server-side DB is always available -->
{@render children()}
