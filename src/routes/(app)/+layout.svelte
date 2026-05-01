<script lang="ts">
	import AppShell from '$lib/components/layout/AppShell.svelte';
	import KeyboardShortcuts from '$lib/components/layout/KeyboardShortcuts.svelte';
	import QuickAddFAB from '$lib/components/ui/QuickAddFAB.svelte';
	import ToastContainer from '$lib/components/layout/ToastContainer.svelte';
	import type { Snippet } from 'svelte';

	const { children }: { children: Snippet } = $props();
</script>

<AppShell>
	{@render children()}
</AppShell>

<KeyboardShortcuts />
<QuickAddFAB />
<ToastContainer />
