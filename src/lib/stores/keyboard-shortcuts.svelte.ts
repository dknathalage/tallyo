import { goto } from '$app/navigation';
import { base } from '$app/paths';

export interface Shortcut {
	key: string;
	label: string;
	i18nKey: string;
	action: () => void;
}

function isFocusInInput(): boolean {
	const el = document.activeElement;
	if (!el) return false;
	const tag = el.tagName.toLowerCase();
	return (
		tag === 'input' ||
		tag === 'textarea' ||
		tag === 'select' ||
		(el as HTMLElement).isContentEditable
	);
}

function focusSearch(): void {
	const input = document.querySelector<HTMLInputElement>(
		'input[type="search"], input[placeholder*="Search"], input[placeholder*="search"]'
	);
	if (input) {
		input.focus();
		input.select();
	}
}

function createShortcutsStore() {
	let showHelp = $state(false);

	const shortcuts: Shortcut[] = [
		{
			key: 'n',
			label: 'N',
			i18nKey: 'shortcuts.newInvoice',
			action: () => goto(`${base}/console/invoices/new`)
		},
		{
			key: 'e',
			label: 'E',
			i18nKey: 'shortcuts.newEstimate',
			action: () => goto(`${base}/console/estimates/new`)
		},
		{
			key: 'c',
			label: 'C',
			i18nKey: 'shortcuts.newClient',
			action: () => goto(`${base}/console/clients/new`)
		},
		{
			key: '/',
			label: '/',
			i18nKey: 'shortcuts.focusSearch',
			action: () => focusSearch()
		},
		{
			key: '?',
			label: '?',
			i18nKey: 'shortcuts.showHelp',
			action: () => {
				showHelp = !showHelp;
			}
		}
	];

	function handleKeydown(e: KeyboardEvent) {
		if (e.ctrlKey || e.altKey || e.metaKey) return;

		if (e.key === '?') {
			if (!isFocusInInput()) {
				e.preventDefault();
				showHelp = !showHelp;
			}
			return;
		}

		if (e.key === 'Escape') {
			if (showHelp) {
				showHelp = false;
			}
			return;
		}

		if (isFocusInInput()) return;

		const shortcut = shortcuts.find((s) => s.key === e.key && s.key !== '?');
		if (shortcut) {
			e.preventDefault();
			shortcut.action();
		}
	}

	return {
		get showHelp() {
			return showHelp;
		},
		set showHelp(value: boolean) {
			showHelp = value;
		},
		shortcuts,
		handleKeydown
	};
}

export const keyboardShortcuts = createShortcutsStore();
