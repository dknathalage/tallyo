import { describe, it, expect, vi, afterEach } from 'vitest';
import { formatCurrency, formatDate, formatDateInput, today } from './format.js';

describe('formatCurrency', () => {
	it('formats positive amounts', () => {
		expect(formatCurrency(1000)).toBe('$1,000.00');
	});

	it('formats zero', () => {
		expect(formatCurrency(0)).toBe('$0.00');
	});

	it('formats decimal amounts', () => {
		expect(formatCurrency(49.99)).toBe('$49.99');
	});

	it('formats large numbers with commas', () => {
		expect(formatCurrency(1234567.89)).toBe('$1,234,567.89');
	});

	it('formats negative amounts', () => {
		expect(formatCurrency(-500)).toBe('-$500.00');
	});

	it('rounds to two decimal places', () => {
		expect(formatCurrency(10.999)).toBe('$11.00');
	});
});

describe('formatDate', () => {
	it('formats a standard date string', () => {
		expect(formatDate('2025-03-24')).toBe('Mar 24, 2025');
	});

	it('formats January date', () => {
		expect(formatDate('2024-01-01')).toBe('Jan 1, 2024');
	});

	it('formats December date', () => {
		expect(formatDate('2024-12-31')).toBe('Dec 31, 2024');
	});
});

describe('formatDateInput', () => {
	it('returns YYYY-MM-DD format', () => {
		expect(formatDateInput('2025-03-24')).toBe('2025-03-24');
	});

	it('pads single-digit month and day', () => {
		expect(formatDateInput('2024-01-05')).toBe('2024-01-05');
	});

	it('handles end of year', () => {
		expect(formatDateInput('2024-12-31')).toBe('2024-12-31');
	});
});

describe('today', () => {
	afterEach(() => {
		vi.useRealTimers();
	});

	it('returns current date in YYYY-MM-DD format', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date(2025, 5, 15)); // June 15, 2025
		expect(today()).toBe('2025-06-15');
	});

	it('pads single-digit month and day', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date(2024, 0, 3)); // Jan 3, 2024
		expect(today()).toBe('2024-01-03');
	});
});
