import { describe, it, expect } from 'vitest';
import { priceFor, periodFor, monthlyPrice, annualPerMonth, annualTotal } from './pricing';

describe('priceFor', () => {
	it('shows the monthly price when annual is false', () => {
		expect(priceFor(false)).toBe(monthlyPrice);
		expect(priceFor(false)).toBe('$19');
	});

	it('shows the annual per-month price when annual is true', () => {
		expect(priceFor(true)).toBe(annualPerMonth);
		expect(priceFor(true)).toBe('$15.83');
	});

	it('annual per-month is cheaper than monthly', () => {
		expect(Number(annualPerMonth.replace('$', ''))).toBeLessThan(
			Number(monthlyPrice.replace('$', ''))
		);
	});

	it('annual total is 10 months of the monthly price (2 months free)', () => {
		expect(Number(annualTotal.replace('$', ''))).toBe(Number(monthlyPrice.replace('$', '')) * 10);
	});
});

describe('periodFor', () => {
	it('reflects the cadence', () => {
		expect(periodFor(false)).toBe('/month');
		expect(periodFor(true)).toBe('/mo, billed annually');
	});
});
