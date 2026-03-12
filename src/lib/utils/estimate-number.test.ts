import { describe, it, expect, vi } from 'vitest';

vi.mock('../db/connection.svelte.js', () => ({
	query: vi.fn()
}));

import { generateEstimateNumber } from './estimate-number.js';
import { query } from '../db/connection.svelte.js';

const mockQuery = vi.mocked(query);

describe('generateEstimateNumber', () => {
	it('returns EST-0001 when no estimates exist', () => {
		mockQuery.mockReturnValue([{ max_num: null }]);
		expect(generateEstimateNumber()).toBe('EST-0001');
	});

	it('returns EST-0001 when query returns empty', () => {
		mockQuery.mockReturnValue([]);
		expect(generateEstimateNumber()).toBe('EST-0001');
	});

	it('increments from the current max estimate number', () => {
		// CAST(SUBSTR('EST-0005', 5) AS INTEGER) = 5
		mockQuery.mockReturnValue([{ max_num: 5 }]);
		expect(generateEstimateNumber()).toBe('EST-0006');
	});

	it('pads the number to 4 digits', () => {
		// CAST(SUBSTR('EST-0009', 5) AS INTEGER) = 9
		mockQuery.mockReturnValue([{ max_num: 9 }]);
		expect(generateEstimateNumber()).toBe('EST-0010');
	});

	it('handles large estimate numbers', () => {
		// CAST(SUBSTR('EST-9999', 5) AS INTEGER) = 9999
		mockQuery.mockReturnValue([{ max_num: 9999 }]);
		expect(generateEstimateNumber()).toBe('EST-10000');
	});
});
