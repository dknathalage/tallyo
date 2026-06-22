<script lang="ts">
	import { page } from '$app/state';
	import ParticipantEditor from '$lib/components/ParticipantEditor.svelte';

	const idParam = $derived((page.params.uuid ?? 'new'));
</script>

<!--
	The id-dependent body lives in ParticipantEditor, instantiated inside
	{#key idParam}, so sibling / back-forward navigation fully remounts it
	(re-seeding fields, resetting the autosave machine and currentId) — consistent
	with the other routes that {#key} their editor on the route id.
-->
{#key idParam}
	<ParticipantEditor {idParam} />
{/key}
