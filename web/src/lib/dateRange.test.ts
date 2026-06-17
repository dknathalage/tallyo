import { describe, it, expect } from 'vitest';
import { toISODate, resolvePreset, PRESET_LABELS, type RangePreset } from './dateRange';

// Build a local-time Date (midnight) so these tests are timezone-safe: resolvePreset
// and toISODate work in local time, and the local-time constructor matches that. We
// pin `today` explicitly via resolvePreset's API rather than relying on the clock.
function localDate(y: number, m1: number, d: number): Date {
	return new Date(y, m1 - 1, d);
}

describe('toISODate', () => {
	it('formats a local date as zero-padded YYYY-MM-DD', () => {
		expect(toISODate(localDate(2026, 6, 9))).toBe('2026-06-09');
		expect(toISODate(localDate(2026, 1, 1))).toBe('2026-01-01');
		expect(toISODate(localDate(2026, 12, 31))).toBe('2026-12-31');
	});

	it('does not drift to the previous day regardless of time-of-day (no UTC off-by-one)', () => {
		// 23:30 local on the 9th must still format as the 9th.
		const lateEvening = new Date(2026, 5, 9, 23, 30, 0);
		expect(toISODate(lateEvening)).toBe('2026-06-09');
		// 00:30 local on the 9th must format as the 9th, not the 8th.
		const earlyMorning = new Date(2026, 5, 9, 0, 30, 0);
		expect(toISODate(earlyMorning)).toBe('2026-06-09');
	});
});

describe('resolvePreset', () => {
	// Anchor on Wednesday 2026-06-10. ISO week (Mon-based) is 2026-06-08 .. 2026-06-14.
	const wed = localDate(2026, 6, 10);

	it('this-week returns the Monday..Sunday week containing today', () => {
		const r = resolvePreset('this-week', wed);
		expect(r).toEqual({ from: '2026-06-08', to: '2026-06-14' });
	});

	it('last-week returns the previous Monday..Sunday week', () => {
		const r = resolvePreset('last-week', wed);
		expect(r).toEqual({ from: '2026-06-01', to: '2026-06-07' });
	});

	it('last-7 returns today and the prior 6 days (inclusive 7-day window)', () => {
		const r = resolvePreset('last-7', wed);
		expect(r).toEqual({ from: '2026-06-04', to: '2026-06-10' });
	});

	it('last-30 returns today and the prior 29 days (inclusive 30-day window)', () => {
		const r = resolvePreset('last-30', wed);
		expect(r).toEqual({ from: '2026-05-12', to: '2026-06-10' });
	});

	it('this-month returns the first..last day of the current month', () => {
		const r = resolvePreset('this-month', wed);
		expect(r).toEqual({ from: '2026-06-01', to: '2026-06-30' });
	});

	it('this-month handles February (28 days in 2026)', () => {
		const r = resolvePreset('this-month', localDate(2026, 2, 15));
		expect(r).toEqual({ from: '2026-02-01', to: '2026-02-28' });
	});

	it('this-week is Monday-based when today is Sunday', () => {
		// Sunday 2026-06-14 belongs to the 2026-06-08..2026-06-14 ISO week.
		const r = resolvePreset('this-week', localDate(2026, 6, 14));
		expect(r).toEqual({ from: '2026-06-08', to: '2026-06-14' });
	});

	it('this-week is Monday-based when today is Monday', () => {
		const r = resolvePreset('this-week', localDate(2026, 6, 8));
		expect(r).toEqual({ from: '2026-06-08', to: '2026-06-14' });
	});

	it('week presets span exactly 7 days (Monday start, Sunday end)', () => {
		for (const preset of ['this-week', 'last-week'] as const) {
			const r = resolvePreset(preset, wed);
			const from = localDate(
				Number(r.from.slice(0, 4)),
				Number(r.from.slice(5, 7)),
				Number(r.from.slice(8, 10))
			);
			const to = localDate(
				Number(r.to.slice(0, 4)),
				Number(r.to.slice(5, 7)),
				Number(r.to.slice(8, 10))
			);
			expect(from.getDay()).toBe(1); // Monday
			expect(to.getDay()).toBe(0); // Sunday
			const days = Math.round((to.getTime() - from.getTime()) / 86_400_000);
			expect(days).toBe(6);
		}
	});

	it('every preset yields from <= to', () => {
		const presets: RangePreset[] = ['this-week', 'last-week', 'last-7', 'last-30', 'this-month'];
		for (const preset of presets) {
			const r = resolvePreset(preset, wed);
			expect(r.from <= r.to).toBe(true);
		}
	});

	it('throws for the custom preset', () => {
		expect(() => resolvePreset('custom', wed)).toThrow();
	});
});

describe('PRESET_LABELS', () => {
	it('has a human label for every non-custom preset and custom', () => {
		expect(PRESET_LABELS['this-week']).toBe('This week');
		expect(PRESET_LABELS['last-week']).toBe('Last week');
		expect(PRESET_LABELS['last-7']).toBe('Last 7 days');
		expect(PRESET_LABELS['last-30']).toBe('Last 30 days');
		expect(PRESET_LABELS['this-month']).toBe('This month');
		expect(PRESET_LABELS['custom']).toBe('Custom range');
	});
});
