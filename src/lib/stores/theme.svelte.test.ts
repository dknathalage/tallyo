import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock document and window before importing the module
const classListMock = {
	add: vi.fn(),
	remove: vi.fn(),
	contains: vi.fn(() => false)
};

const mockMatchMedia = vi.fn((query: string) => ({
	matches: false,
	media: query,
	addEventListener: vi.fn(),
	removeEventListener: vi.fn()
}));

Object.defineProperty(globalThis, 'document', {
	value: { documentElement: { classList: classListMock } },
	writable: true
});

Object.defineProperty(globalThis, 'window', {
	value: { matchMedia: mockMatchMedia },
	writable: true
});

const localStorageMock = (() => {
	let store: Record<string, string> = {};
	return {
		getItem: vi.fn((key: string) => store[key] ?? null),
		setItem: vi.fn((key: string, val: string) => { store[key] = val; }),
		removeItem: vi.fn((key: string) => { delete store[key]; }),
		clear: vi.fn(() => { store = {}; })
	};
})();

Object.defineProperty(globalThis, 'localStorage', {
	value: localStorageMock,
	writable: true
});

import { theme } from './theme.svelte.js';

beforeEach(() => {
	vi.clearAllMocks();
	localStorageMock.clear();
	// Reset matchMedia to return false by default
	mockMatchMedia.mockImplementation((query: string) => ({
		matches: false,
		media: query,
		addEventListener: vi.fn(),
		removeEventListener: vi.fn()
	}));
});

describe('theme store', () => {
	it('has preference getter', () => {
		expect(theme.preference).toBeDefined();
	});

	it('has isDark getter', () => {
		expect(typeof theme.isDark).toBe('boolean');
	});

	it('set() changes preference to light', () => {
		theme.set('light');
		expect(theme.preference).toBe('light');
	});

	it('set() changes preference to dark', () => {
		theme.set('dark');
		expect(theme.preference).toBe('dark');
	});

	it('set() saves to localStorage', () => {
		theme.set('dark');
		expect(localStorageMock.setItem).toHaveBeenCalledWith('theme', 'dark');
	});

	it('set("dark") makes isDark true', () => {
		theme.set('dark');
		expect(theme.isDark).toBe(true);
	});

	it('set("light") makes isDark false', () => {
		theme.set('light');
		expect(theme.isDark).toBe(false);
	});

	it('set("dark") adds dark class to documentElement', () => {
		theme.set('dark');
		expect(classListMock.add).toHaveBeenCalledWith('dark');
	});

	it('set("light") removes dark class from documentElement', () => {
		theme.set('light');
		expect(classListMock.remove).toHaveBeenCalledWith('dark');
	});

	it('toggle() switches from light to dark', () => {
		theme.set('light');
		theme.toggle();
		expect(theme.preference).toBe('dark');
	});

	it('toggle() switches from dark to light', () => {
		theme.set('dark');
		theme.toggle();
		expect(theme.preference).toBe('light');
	});

	it('init() reads from localStorage', () => {
		localStorageMock.getItem.mockReturnValueOnce('dark');
		theme.init();
		expect(theme.preference).toBe('dark');
	});

	it('init() does not change preference when localStorage has no value', () => {
		// Set to a known value first
		theme.set('light');
		localStorageMock.getItem.mockReturnValueOnce(null as any);
		mockMatchMedia.mockReturnValue({
			matches: false,
			media: '',
			addEventListener: vi.fn(),
			removeEventListener: vi.fn()
		});
		theme.init();
		// When no stored value, preference stays as 'light' (unchanged)
		expect(theme.preference).toBe('light');
	});

	it('init() ignores invalid stored value', () => {
		localStorageMock.getItem.mockReturnValueOnce('invalid-theme');
		theme.init();
		// preference stays as whatever it was before init (system)
		// The stored 'invalid-theme' is not 'light' | 'dark' | 'system' so it is not applied
		// But the previous state might be 'dark' from earlier tests, so just check it's valid
		const valid = ['light', 'dark', 'system'];
		expect(valid).toContain(theme.preference);
	});

	it('init() applies dark when system prefers dark', () => {
		localStorageMock.getItem.mockReturnValueOnce(null as any);
		mockMatchMedia.mockReturnValue({
			matches: true,
			media: '',
			addEventListener: vi.fn(),
			removeEventListener: vi.fn()
		});
		// First reset to system preference
		theme.set('system');
		theme.init();
		expect(theme.isDark).toBe(true);
	});

	it('set("system") resolves system preference', () => {
		mockMatchMedia.mockReturnValue({
			matches: false,
			media: '',
			addEventListener: vi.fn(),
			removeEventListener: vi.fn()
		});
		theme.set('system');
		expect(theme.preference).toBe('system');
		expect(theme.isDark).toBe(false);
	});
});
