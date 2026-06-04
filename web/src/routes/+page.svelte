<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	onMount(() => {
		void goto('/settings');
	});
</script>

<p class="text-sm text-gray-500">Redirecting…</p>
