<script lang="ts">
	import { keyboardShortcuts } from '$lib/stores/keyboard-shortcuts.svelte.js';
	import KeyboardShortcutsModal from '$lib/components/ui/KeyboardShortcutsModal.svelte';
</script>

<svelte:window onkeydown={keyboardShortcuts.handleKeydown} />

<KeyboardShortcutsModal
	open={keyboardShortcuts.showHelp}
	onclose={() => (keyboardShortcuts.showHelp = false)}
/>
