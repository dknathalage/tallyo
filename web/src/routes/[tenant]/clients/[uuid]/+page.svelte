<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/nav';
	import ClientEditor from '$lib/components/ClientEditor.svelte';

	const idParam = $derived((page.params.uuid ?? 'new'));

	// Creation is modal-only (from the clients list); a stray /clients/new redirects.
	$effect(() => {
		if (idParam === 'new') void goto(t('/clients'));
	});
</script>

<!--
	The id-dependent body lives in ClientEditor, instantiated inside
	{#key idParam}, so sibling / back-forward navigation fully remounts it
	(re-seeding fields, resetting the autosave machine and currentId) — consistent
	with the other routes that {#key} their editor on the route id.
-->
{#key idParam}
	{#if idParam !== 'new'}
		<ClientEditor {idParam} />
	{/if}
{/key}
