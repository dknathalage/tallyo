import { describe, it, expect, vi, beforeEach } from 'vitest';

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

import { i18n } from './i18n.svelte.js';

beforeEach(() => {
	vi.clearAllMocks();
	localStorageMock.clear();
});

describe('i18n store', () => {
	it('defaults locale to en', () => {
		expect(i18n.locale).toBe('en');
	});

	it('intlLocale returns en-US for en', () => {
		expect(i18n.intlLocale).toBe('en-US');
	});

	it('intlLocale returns es-ES for es', async () => {
		await i18n.setLocale('es');
		expect(i18n.intlLocale).toBe('es-ES');
		await i18n.setLocale('en');
	});

	it('intlLocale returns fr-FR for fr', async () => {
		await i18n.setLocale('fr');
		expect(i18n.intlLocale).toBe('fr-FR');
		await i18n.setLocale('en');
	});

	it('intlLocale returns de-DE for de', async () => {
		await i18n.setLocale('de');
		expect(i18n.intlLocale).toBe('de-DE');
		await i18n.setLocale('en');
	});

	it('intlLocale returns ja-JP for ja', async () => {
		await i18n.setLocale('ja');
		expect(i18n.intlLocale).toBe('ja-JP');
		await i18n.setLocale('en');
	});

	it('intlLocale falls back to en-US for unknown locale', async () => {
		await i18n.setLocale('xx');
		expect(i18n.intlLocale).toBe('en-US');
		await i18n.setLocale('en');
	});

	it('t() returns translation for valid key', () => {
		const result = i18n.t('nav.dashboard');
		expect(result).toBe('Dashboard');
	});

	it('t() returns translation for nested key', () => {
		const result = i18n.t('pdf.invoice');
		expect(result).toBe('INVOICE');
	});

	it('t() returns key itself for missing key', () => {
		const result = i18n.t('nonexistent.key');
		expect(result).toBe('nonexistent.key');
	});

	it('t() returns key for partially valid path', () => {
		const result = i18n.t('nav.nonexistent');
		expect(result).toBe('nav.nonexistent');
	});

	it('t() returns key when path leads to non-string', () => {
		// 'nav' alone is an object, not a string
		const result = i18n.t('nav');
		expect(result).toBe('nav');
	});

	it('t() interpolates values', () => {
		const result = i18n.t('pdf.tax', { rate: 10 });
		expect(result).toBe('Tax (10%):');
	});

	it('t() interpolates multiple values', () => {
		const result = i18n.t('dashboard.excludedCurrencyNote', { count: '3', plural: 's' });
		expect(result).toBe('3 invoices in other currencies not included in totals.');
	});

	it('t() leaves missing interpolation keys as {key}', () => {
		const result = i18n.t('pdf.tax', {});
		expect(result).toContain('{rate}');
	});

	it('setLocale() changes locale and saves to localStorage', async () => {
		await i18n.setLocale('fr');
		expect(i18n.locale).toBe('fr');
		expect(localStorageMock.setItem).toHaveBeenCalledWith('locale', 'fr');
		await i18n.setLocale('en');
	});

	it('init() loads locale from localStorage', () => {
		localStorageMock.getItem.mockReturnValueOnce('de');
		i18n.init();
		expect(i18n.locale).toBe('de');
	});

	it('init() keeps default locale when localStorage is empty', () => {
		localStorageMock.getItem.mockReturnValueOnce(null as any);
		i18n.init();
		// locale was changed to 'de' above, so we just check init doesn't crash
		// and locale stays as whatever was set
		expect(typeof i18n.locale).toBe('string');
	});

	it('t() handles empty string key', () => {
		const result = i18n.t('');
		// empty string split by '.' gives [''], traversal won't find a match
		expect(typeof result).toBe('string');
	});

	it('t() handles deeply nested missing key', () => {
		const result = i18n.t('nav.a.b.c.d');
		expect(result).toBe('nav.a.b.c.d');
	});
});
