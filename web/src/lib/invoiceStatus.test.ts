import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { isOverdue, effectiveStatus } from './invoiceStatus';

describe('isOverdue', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-06-15T12:00:00Z'));
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it('past-due sent is overdue', () => {
		expect(isOverdue('sent', '2026-06-01')).toBe(true);
	});

	it('future sent is not overdue', () => {
		expect(isOverdue('sent', '2026-07-01')).toBe(false);
	});

	it('today sent is not overdue', () => {
		expect(isOverdue('sent', '2026-06-15')).toBe(false);
	});

	it('draft is never overdue even if past due', () => {
		expect(isOverdue('draft', '2026-06-01')).toBe(false);
	});

	it('paid is never overdue even if past due', () => {
		expect(isOverdue('paid', '2026-06-01')).toBe(false);
	});

	it('null dueDate is not overdue', () => {
		expect(isOverdue('sent', null)).toBe(false);
	});

	it('undefined dueDate is not overdue', () => {
		expect(isOverdue('sent', undefined)).toBe(false);
	});

	it('empty-string dueDate is not overdue', () => {
		expect(isOverdue('sent', '')).toBe(false);
	});

	it('ISO time-of-day noise still overdue (compares YYYY-MM-DD prefix)', () => {
		expect(isOverdue('sent', '2000-01-01T23:59:59Z')).toBe(true);
	});
});

describe('effectiveStatus', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-06-15T12:00:00Z'));
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it('promotes past-due sent to overdue', () => {
		expect(effectiveStatus('sent', '2026-06-01')).toBe('overdue');
	});

	it('passes through draft unchanged', () => {
		expect(effectiveStatus('draft', '2026-06-01')).toBe('draft');
	});

	it('passes through paid unchanged', () => {
		expect(effectiveStatus('paid', '2026-06-01')).toBe('paid');
	});

	it('passes through future sent unchanged', () => {
		expect(effectiveStatus('sent', '2026-07-01')).toBe('sent');
	});
});
