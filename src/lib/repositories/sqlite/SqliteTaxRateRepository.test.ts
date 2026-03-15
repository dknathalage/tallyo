import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/tax-rates.js', () => ({
	getTaxRates: vi.fn(),
	getDefaultTaxRate: vi.fn(),
	getTaxRate: vi.fn(),
	createTaxRate: vi.fn(),
	updateTaxRate: vi.fn(),
	deleteTaxRate: vi.fn()
}));

import { SqliteTaxRateRepository } from './SqliteTaxRateRepository.js';
import * as queries from '$lib/db/queries/tax-rates.js';

const mockGetTaxRates = vi.mocked(queries.getTaxRates);
const mockGetDefaultTaxRate = vi.mocked(queries.getDefaultTaxRate);
const mockGetTaxRate = vi.mocked(queries.getTaxRate);
const mockCreateTaxRate = vi.mocked(queries.createTaxRate);
const mockUpdateTaxRate = vi.mocked(queries.updateTaxRate);
const mockDeleteTaxRate = vi.mocked(queries.deleteTaxRate);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('SqliteTaxRateRepository', () => {
	describe('getTaxRates', () => {
		it('delegates to getTaxRates query', () => {
			const repo = new SqliteTaxRateRepository();
			const rates = [{ id: 1, name: 'GST', rate: 10 }] as any;
			mockGetTaxRates.mockReturnValue(rates);

			const result = repo.getTaxRates();
			expect(mockGetTaxRates).toHaveBeenCalled();
			expect(result).toBe(rates);
		});
	});

	describe('getDefaultTaxRate', () => {
		it('delegates to getDefaultTaxRate query', () => {
			const repo = new SqliteTaxRateRepository();
			const rate = { id: 1, name: 'GST', rate: 10, is_default: true } as any;
			mockGetDefaultTaxRate.mockReturnValue(rate);

			expect(repo.getDefaultTaxRate()).toBe(rate);
		});

		it('returns null when no default', () => {
			const repo = new SqliteTaxRateRepository();
			mockGetDefaultTaxRate.mockReturnValue(null);
			expect(repo.getDefaultTaxRate()).toBeNull();
		});
	});

	describe('getTaxRate', () => {
		it('delegates to getTaxRate query', () => {
			const repo = new SqliteTaxRateRepository();
			const rate = { id: 1, name: 'GST', rate: 10 } as any;
			mockGetTaxRate.mockReturnValue(rate);

			expect(repo.getTaxRate(1)).toBe(rate);
			expect(mockGetTaxRate).toHaveBeenCalledWith(1);
		});

		it('returns null when not found', () => {
			const repo = new SqliteTaxRateRepository();
			mockGetTaxRate.mockReturnValue(null);
			expect(repo.getTaxRate(999)).toBeNull();
		});
	});

	describe('createTaxRate', () => {
		it('delegates to createTaxRate query', async () => {
			const repo = new SqliteTaxRateRepository();
			mockCreateTaxRate.mockResolvedValue(2);

			const data = { name: 'VAT', rate: 20, is_default: false };
			const id = await repo.createTaxRate(data);

			expect(mockCreateTaxRate).toHaveBeenCalledWith(data);
			expect(id).toBe(2);
		});
	});

	describe('updateTaxRate', () => {
		it('delegates to updateTaxRate query', async () => {
			const repo = new SqliteTaxRateRepository();
			mockUpdateTaxRate.mockResolvedValue(undefined);

			const data = { name: 'GST Updated', rate: 15, is_default: true };
			await repo.updateTaxRate(1, data);

			expect(mockUpdateTaxRate).toHaveBeenCalledWith(1, data);
		});
	});

	describe('deleteTaxRate', () => {
		it('delegates to deleteTaxRate query', async () => {
			const repo = new SqliteTaxRateRepository();
			mockDeleteTaxRate.mockResolvedValue(undefined);

			await repo.deleteTaxRate(3);
			expect(mockDeleteTaxRate).toHaveBeenCalledWith(3);
		});

		it('propagates errors', async () => {
			const repo = new SqliteTaxRateRepository();
			mockDeleteTaxRate.mockRejectedValue(new Error('cannot delete default'));

			await expect(repo.deleteTaxRate(1)).rejects.toThrow('cannot delete default');
		});
	});
});
