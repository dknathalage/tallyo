import { describe, it, expect } from 'vitest';
import { getCurrencyInfo, CURRENCIES } from './currency.js';

describe('CURRENCIES', () => {
	it('contains at least one entry', () => {
		expect(CURRENCIES.length).toBeGreaterThan(0);
	});

	it('first entry is USD', () => {
		expect(CURRENCIES[0].code).toBe('USD');
	});

	it('every entry has required fields', () => {
		for (const c of CURRENCIES) {
			expect(c).toHaveProperty('code');
			expect(c).toHaveProperty('name');
			expect(c).toHaveProperty('symbol');
			expect(c).toHaveProperty('decimals');
			expect(typeof c.decimals).toBe('number');
		}
	});

	it('JPY has 0 decimals', () => {
		const jpy = CURRENCIES.find((c) => c.code === 'JPY');
		expect(jpy?.decimals).toBe(0);
	});

	it('KRW has 0 decimals', () => {
		const krw = CURRENCIES.find((c) => c.code === 'KRW');
		expect(krw?.decimals).toBe(0);
	});

	it('EUR has 2 decimals', () => {
		const eur = CURRENCIES.find((c) => c.code === 'EUR');
		expect(eur?.decimals).toBe(2);
	});
});

describe('getCurrencyInfo', () => {
	it('returns USD info for USD code', () => {
		const info = getCurrencyInfo('USD');
		expect(info.code).toBe('USD');
		expect(info.symbol).toBe('$');
		expect(info.name).toBe('US Dollar');
		expect(info.decimals).toBe(2);
	});

	it('returns EUR info for EUR code', () => {
		const info = getCurrencyInfo('EUR');
		expect(info.code).toBe('EUR');
		expect(info.symbol).toBe('€');
	});

	it('returns GBP info for GBP code', () => {
		const info = getCurrencyInfo('GBP');
		expect(info.code).toBe('GBP');
		expect(info.symbol).toBe('£');
	});

	it('returns JPY info for JPY code', () => {
		const info = getCurrencyInfo('JPY');
		expect(info.code).toBe('JPY');
		expect(info.decimals).toBe(0);
	});

	it('falls back to USD for unknown currency code', () => {
		const info = getCurrencyInfo('XYZ');
		expect(info.code).toBe('USD');
	});

	it('falls back to USD for empty string', () => {
		const info = getCurrencyInfo('');
		expect(info.code).toBe('USD');
	});

	it('is case-sensitive - lowercase returns USD fallback', () => {
		const info = getCurrencyInfo('usd');
		expect(info.code).toBe('USD');
	});

	it('returns AED info', () => {
		const info = getCurrencyInfo('AED');
		expect(info.code).toBe('AED');
		expect(info.name).toBe('UAE Dirham');
	});

	it('returns CHF info', () => {
		const info = getCurrencyInfo('CHF');
		expect(info.code).toBe('CHF');
		expect(info.symbol).toBe('CHF');
	});

	it('returns all 20 currencies', () => {
		expect(CURRENCIES).toHaveLength(20);
	});
});
