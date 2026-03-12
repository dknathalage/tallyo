<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import Modal from '$lib/components/shared/Modal.svelte';
	import { i18n } from '$lib/stores/i18n.svelte.js';

	let showHelp = $state(false);

	/** Returns true when focus is inside a text-entry element (shortcut should be suppressed). */
	function isFocusInInput(): boolean {
		const el = document.activeElement;
		if (!el) return false;
		const tag = el.tagName.toLowerCase();
		return tag === 'input' || tag === 'textarea' || tag === 'select' || (el as HTMLElement).isContentEditable;
	}

	function getNewRoute(): string | null {
		const path = page.url.pathname;
		if (path.includes('/console/invoices') && !path.includes('/invoices/')) return `${base}/console/invoices/new`;
		if (path.includes('/console/estimates') && !path.includes('/estimates/')) return `${base}/console/estimates/new`;
		if (path.includes('/console/clients') && !path.includes('/clients/')) return `${base}/console/clients/new`;
		return null;
	}

	function focusSearch(): void {
		const input = document.querySelector<HTMLInputElement>('input[type="search"], input[placeholder*="Search"], input[placeholder*="search"]');
		if (input) {
			input.focus();
			input.select();
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		// Never fire when typing in a field or if modifier keys are held (except Shift for ?)
		if (e.ctrlKey || e.altKey || e.metaKey) return;

		// ? key (Shift+/) opens help
		if (e.key === '?' && !isFocusInInput()) {
			e.preventDefault();
			showHelp = !showHelp;
			return;
		}

		// Escape closes help (Modal handles its own Escape, but handle top-level help here too)
		if (e.key === 'Escape') {
			if (showHelp) {
				showHelp = false;
			}
			return;
		}

		// All remaining shortcuts are suppressed when typing in a field
		if (isFocusInInput()) return;

		if (e.key === 'n') {
			const route = getNewRoute();
			if (route) {
				e.preventDefault();
				goto(route);
			}
		} else if (e.key === '/') {
			e.preventDefault();
			focusSearch();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<Modal open={showHelp} onclose={() => (showHelp = false)} title={i18n.t('shortcuts.title')}>
	<table class="w-full text-sm">
		<tbody class="divide-y divide-gray-100 dark:divide-gray-700">
			<tr class="py-2">
				<td class="py-2 pr-4">
					<kbd class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-gray-700">n</kbd>
				</td>
				<td class="py-2 text-gray-600 dark:text-gray-300">{i18n.t('shortcuts.newItem')}</td>
			</tr>
			<tr class="py-2">
				<td class="py-2 pr-4">
					<kbd class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-gray-700">/</kbd>
				</td>
				<td class="py-2 text-gray-600 dark:text-gray-300">{i18n.t('shortcuts.focusSearch')}</td>
			</tr>
			<tr class="py-2">
				<td class="py-2 pr-4">
					<kbd class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-gray-700">Esc</kbd>
				</td>
				<td class="py-2 text-gray-600 dark:text-gray-300">{i18n.t('shortcuts.closeModal')}</td>
			</tr>
			<tr class="py-2">
				<td class="py-2 pr-4">
					<kbd class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs dark:bg-gray-700">?</kbd>
				</td>
				<td class="py-2 text-gray-600 dark:text-gray-300">{i18n.t('shortcuts.showHelp')}</td>
			</tr>
		</tbody>
	</table>
</Modal>
