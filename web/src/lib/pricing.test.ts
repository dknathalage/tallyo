import { describe, it, expect } from 'vitest';
import { pricesFor, monthlyPrices, annualPrices } from './pricing';

describe('pricesFor', () => {
	it('returns monthly prices when annual is false', () => {
		expect(pricesFor(false)).toEqual(monthlyPrices);
		expect(pricesFor(false).starter).toBe('$0');
		expect(pricesFor(false).professional).toBe('$29');
		expect(pricesFor(false).business).toBe('$79');
	});

	it('returns annual prices when annual is true', () => {
		expect(pricesFor(true)).toEqual(annualPrices);
		expect(pricesFor(true).starter).toBe('$0');
		expect(pricesFor(true).professional).toBe('$24');
		expect(pricesFor(true).business).toBe('$66');
	});

	it('annual is never pricier than monthly for paid tiers', () => {
		for (const tier of ['professional', 'business'] as const) {
			const m = Number(monthlyPrices[tier].replace('$', ''));
			const a = Number(annualPrices[tier].replace('$', ''));
			expect(a).toBeLessThan(m);
		}
	});
});
