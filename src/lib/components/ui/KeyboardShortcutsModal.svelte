<script lang="ts">
	import Modal from '$lib/components/shared/Modal.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';
	import { keyboardShortcuts } from '$lib/stores/keyboard-shortcuts.svelte.js';

	const {
		open = false,
		onclose
	}: {
		open: boolean;
		onclose: () => void;
	} = $props();
</script>

<Modal {open} {onclose} title={i18n.t('shortcuts.title')}>
	<table class="w-full text-sm">
		<tbody class="divide-y divide-gray-100 dark:divide-gray-700">
			{#each keyboardShortcuts.shortcuts as shortcut (shortcut.label)}
				<tr class="py-2">
					<td class="py-2 pr-4">
						<kbd
							class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-gray-700"
						>{shortcut.label}</kbd>
					</td>
					<td class="py-2 text-gray-600 dark:text-gray-300"
						>{i18n.t(shortcut.i18nKey)}</td
					>
				</tr>
			{/each}
			<tr class="py-2">
				<td class="py-2 pr-4">
					<kbd
						class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-gray-700"
					>Esc</kbd>
				</td>
				<td class="py-2 text-gray-600 dark:text-gray-300"
					>{i18n.t('shortcuts.closeModal')}</td
				>
			</tr>
		</tbody>
	</table>
</Modal>
