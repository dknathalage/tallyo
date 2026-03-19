<script lang="ts">
	import { announcer } from '$lib/stores/announcer.svelte.js';
</script>

<!-- Polite announcements (queued, non-interrupting) -->
<div
	role="status"
	aria-live="polite"
	aria-atomic="true"
	class="sr-only"
>
	{announcer.politeMessage}
</div>

<!-- Assertive announcements (immediate, interrupting) -->
<div
	role="alert"
	aria-live="assertive"
	aria-atomic="true"
	class="sr-only"
>
	{announcer.assertiveMessage}
</div>

<style>
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border-width: 0;
	}
</style>
