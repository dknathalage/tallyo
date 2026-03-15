import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock $app/navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$app/paths', () => ({
	base: ''
}));

// Set up document mocks before import
const mockActiveElement = { tagName: 'BODY', isContentEditable: false };
Object.defineProperty(globalThis, 'document', {
	value: {
		activeElement: mockActiveElement,
		querySelector: vi.fn(() => null),
		documentElement: { classList: { add: vi.fn(), remove: vi.fn() } }
	},
	writable: true
});

import { keyboardShortcuts } from './keyboard-shortcuts.svelte.js';
import { goto } from '$app/navigation';

const mockGoto = vi.mocked(goto);

function makeKeyEvent(key: string, opts: Partial<KeyboardEvent> = {}): KeyboardEvent {
	return {
		key,
		ctrlKey: false,
		altKey: false,
		metaKey: false,
		preventDefault: vi.fn(),
		...opts
	} as unknown as KeyboardEvent;
}

beforeEach(() => {
	vi.clearAllMocks();
	keyboardShortcuts.showHelp = false;
	// Reset document.activeElement to a non-input element
	(document as any).activeElement = { tagName: 'BODY', isContentEditable: false };
	(document as any).querySelector = vi.fn(() => null);
});

describe('keyboardShortcuts store', () => {
	it('showHelp starts as false', () => {
		expect(keyboardShortcuts.showHelp).toBe(false);
	});

	it('shortcuts array is non-empty', () => {
		expect(keyboardShortcuts.shortcuts.length).toBeGreaterThan(0);
	});

	it('shortcuts include n, e, c, /, ? keys', () => {
		const keys = keyboardShortcuts.shortcuts.map((s) => s.key);
		expect(keys).toContain('n');
		expect(keys).toContain('e');
		expect(keys).toContain('c');
		expect(keys).toContain('/');
		expect(keys).toContain('?');
	});

	it('pressing ? toggles showHelp to true', () => {
		keyboardShortcuts.handleKeydown(makeKeyEvent('?'));
		expect(keyboardShortcuts.showHelp).toBe(true);
	});

	it('pressing ? again toggles showHelp back to false', () => {
		keyboardShortcuts.showHelp = true;
		keyboardShortcuts.handleKeydown(makeKeyEvent('?'));
		expect(keyboardShortcuts.showHelp).toBe(false);
	});

	it('pressing Escape closes help when open', () => {
		keyboardShortcuts.showHelp = true;
		keyboardShortcuts.handleKeydown(makeKeyEvent('Escape'));
		expect(keyboardShortcuts.showHelp).toBe(false);
	});

	it('pressing Escape does nothing when help is closed', () => {
		keyboardShortcuts.showHelp = false;
		keyboardShortcuts.handleKeydown(makeKeyEvent('Escape'));
		expect(keyboardShortcuts.showHelp).toBe(false);
	});

	it('pressing n navigates to new invoice', () => {
		keyboardShortcuts.handleKeydown(makeKeyEvent('n'));
		expect(mockGoto).toHaveBeenCalledWith('/console/invoices/new');
	});

	it('pressing e navigates to new estimate', () => {
		keyboardShortcuts.handleKeydown(makeKeyEvent('e'));
		expect(mockGoto).toHaveBeenCalledWith('/console/estimates/new');
	});

	it('pressing c navigates to new client', () => {
		keyboardShortcuts.handleKeydown(makeKeyEvent('c'));
		expect(mockGoto).toHaveBeenCalledWith('/console/clients/new');
	});

	it('pressing / focuses search input when one exists', () => {
		const mockInput = { focus: vi.fn(), select: vi.fn() };
		(document as any).querySelector = vi.fn(() => mockInput);
		keyboardShortcuts.handleKeydown(makeKeyEvent('/'));
		expect(mockInput.focus).toHaveBeenCalled();
		expect(mockInput.select).toHaveBeenCalled();
	});

	it('pressing / does nothing when no search input found', () => {
		(document as any).querySelector = vi.fn(() => null);
		expect(() => keyboardShortcuts.handleKeydown(makeKeyEvent('/'))).not.toThrow();
	});

	it('ignores events with ctrlKey', () => {
		keyboardShortcuts.handleKeydown(makeKeyEvent('n', { ctrlKey: true }));
		expect(mockGoto).not.toHaveBeenCalled();
	});

	it('ignores events with altKey', () => {
		keyboardShortcuts.handleKeydown(makeKeyEvent('n', { altKey: true }));
		expect(mockGoto).not.toHaveBeenCalled();
	});

	it('ignores events with metaKey', () => {
		keyboardShortcuts.handleKeydown(makeKeyEvent('n', { metaKey: true }));
		expect(mockGoto).not.toHaveBeenCalled();
	});

	it('ignores shortcut when focused in input element', () => {
		(document as any).activeElement = { tagName: 'INPUT', isContentEditable: false };
		keyboardShortcuts.handleKeydown(makeKeyEvent('n'));
		expect(mockGoto).not.toHaveBeenCalled();
	});

	it('ignores shortcut when focused in textarea', () => {
		(document as any).activeElement = { tagName: 'TEXTAREA', isContentEditable: false };
		keyboardShortcuts.handleKeydown(makeKeyEvent('e'));
		expect(mockGoto).not.toHaveBeenCalled();
	});

	it('ignores shortcut when focused in select', () => {
		(document as any).activeElement = { tagName: 'SELECT', isContentEditable: false };
		keyboardShortcuts.handleKeydown(makeKeyEvent('c'));
		expect(mockGoto).not.toHaveBeenCalled();
	});

	it('ignores shortcut when focused in contenteditable element', () => {
		(document as any).activeElement = { tagName: 'DIV', isContentEditable: true };
		keyboardShortcuts.handleKeydown(makeKeyEvent('n'));
		expect(mockGoto).not.toHaveBeenCalled();
	});

	it('? key prevents default', () => {
		const event = makeKeyEvent('?');
		keyboardShortcuts.handleKeydown(event);
		expect(event.preventDefault).toHaveBeenCalled();
	});

	it('n key prevents default', () => {
		const event = makeKeyEvent('n');
		keyboardShortcuts.handleKeydown(event);
		expect(event.preventDefault).toHaveBeenCalled();
	});

	it('? in input does not toggle showHelp', () => {
		(document as any).activeElement = { tagName: 'INPUT', isContentEditable: false };
		keyboardShortcuts.handleKeydown(makeKeyEvent('?'));
		expect(keyboardShortcuts.showHelp).toBe(false);
	});

	it('showHelp can be set directly', () => {
		keyboardShortcuts.showHelp = true;
		expect(keyboardShortcuts.showHelp).toBe(true);
		keyboardShortcuts.showHelp = false;
		expect(keyboardShortcuts.showHelp).toBe(false);
	});

	it('each shortcut has required fields', () => {
		for (const s of keyboardShortcuts.shortcuts) {
			expect(s).toHaveProperty('key');
			expect(s).toHaveProperty('label');
			expect(s).toHaveProperty('i18nKey');
			expect(typeof s.action).toBe('function');
		}
	});
});
