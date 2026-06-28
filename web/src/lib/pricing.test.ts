import { describe, it, expect } from 'vitest';
import { planFor, monthlyPrice, annualPerMonth } from './pricing';

describe('planFor', () => {
	it('returns the monthly price when annual is false', () => {
		expect(planFor(false)).toEqual({ price: monthlyPrice, period: '/month' });
	});

	it('returns the annual per-month price when annual is true', () => {
		expect(planFor(true)).toEqual({ price: annualPerMonth, period: '/mo, billed annually' });
	});

	it('annual per-month is cheaper than monthly', () => {
		const m = Number(monthlyPrice.replace('$', ''));
		const a = Number(annualPerMonth.replace('$', ''));
		expect(a).toBeLessThan(m);
	});
});
