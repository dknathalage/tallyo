import { describe, it, expect, vi } from 'vitest';

vi.mock('../db/connection.svelte.js', () => ({
	query: vi.fn()
}));

import { generateInvoiceNumber } from './invoice-number.js';
import { query } from '../db/connection.svelte.js';

const mockQuery = vi.mocked(query);

describe('generateInvoiceNumber', () => {
	it('returns INV-0001 when no invoices exist (null result)', () => {
		mockQuery.mockReturnValue([{ max_num: null }]);
		expect(generateInvoiceNumber()).toBe('INV-0001');
	});

	it('returns INV-0001 when query returns empty array', () => {
		mockQuery.mockReturnValue([]);
		expect(generateInvoiceNumber()).toBe('INV-0001');
	});

	it('increments from the current max invoice number', () => {
		mockQuery.mockReturnValue([{ max_num: 5 }]);
		expect(generateInvoiceNumber()).toBe('INV-0006');
	});

	it('pads the number to 4 digits', () => {
		mockQuery.mockReturnValue([{ max_num: 9 }]);
		expect(generateInvoiceNumber()).toBe('INV-0010');
	});

	it('handles large invoice numbers beyond 4 digits', () => {
		mockQuery.mockReturnValue([{ max_num: 9999 }]);
		expect(generateInvoiceNumber()).toBe('INV-10000');
	});

	it('uses numeric ordering — picks the highest numeric value, not lexicographic max', () => {
		// If the DB contained INV-0002 and INV-0010, lexicographic MAX gives 'INV-0002'
		// (because '2' > '1'). Our fix returns the true numeric max so the next number
		// is INV-0011, not INV-0003.
		mockQuery.mockReturnValue([{ max_num: 10 }]);
		expect(generateInvoiceNumber()).toBe('INV-0011');
	});

	it('returns INV-0001 when only non-standard invoice numbers exist', () => {
		// Non-standard numbers like 'INV-CUSTOM' are filtered out by the GLOB clause
		// so max_num is null — we must never produce INV-NaN.
		mockQuery.mockReturnValue([{ max_num: null }]);
		expect(generateInvoiceNumber()).toBe('INV-0001');
	});

	it('uses the correct SQL with GLOB filter and numeric CAST', () => {
		mockQuery.mockReturnValue([{ max_num: null }]);
		generateInvoiceNumber();

		const [calledSql] = mockQuery.mock.calls[mockQuery.mock.calls.length - 1];
		expect(calledSql).toContain("GLOB 'INV-[0-9]*'");
		expect(calledSql).toContain('CAST(SUBSTR(invoice_number, 5) AS INTEGER)');
	});
});
