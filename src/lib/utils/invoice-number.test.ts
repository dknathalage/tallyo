import { describe, it, expect, vi } from 'vitest';

vi.mock('../db/connection.svelte.js', () => ({
	query: vi.fn()
}));

import { generateInvoiceNumber } from './invoice-number.js';
import { query } from '../db/connection.svelte.js';

const mockQuery = vi.mocked(query);

describe('generateInvoiceNumber', () => {
	it('returns INV-0001 when no invoices exist', () => {
		mockQuery.mockReturnValue([{ max_num: null }]);
		expect(generateInvoiceNumber()).toBe('INV-0001');
	});

	it('returns INV-0001 when query returns empty', () => {
		mockQuery.mockReturnValue([]);
		expect(generateInvoiceNumber()).toBe('INV-0001');
	});

	it('increments from the current max invoice number', () => {
		mockQuery.mockReturnValue([{ max_num: 'INV-0005' }]);
		expect(generateInvoiceNumber()).toBe('INV-0006');
	});

	it('pads the number to 4 digits', () => {
		mockQuery.mockReturnValue([{ max_num: 'INV-0009' }]);
		expect(generateInvoiceNumber()).toBe('INV-0010');
	});

	it('handles large invoice numbers', () => {
		mockQuery.mockReturnValue([{ max_num: 'INV-9999' }]);
		expect(generateInvoiceNumber()).toBe('INV-10000');
	});
});
