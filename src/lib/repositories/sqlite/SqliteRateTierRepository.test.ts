import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/rate-tiers.js', () => ({
	getRateTiers: vi.fn(),
	getRateTier: vi.fn(),
	getDefaultTier: vi.fn(),
	createRateTier: vi.fn(),
	updateRateTier: vi.fn(),
	deleteRateTier: vi.fn()
}));

import { SqliteRateTierRepository } from './SqliteRateTierRepository.js';
import * as queries from '$lib/db/queries/rate-tiers.js';

const mockGetRateTiers = vi.mocked(queries.getRateTiers);
const mockGetRateTier = vi.mocked(queries.getRateTier);
const mockGetDefaultTier = vi.mocked(queries.getDefaultTier);
const mockCreateRateTier = vi.mocked(queries.createRateTier);
const mockUpdateRateTier = vi.mocked(queries.updateRateTier);
const mockDeleteRateTier = vi.mocked(queries.deleteRateTier);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('SqliteRateTierRepository', () => {
	describe('getRateTiers', () => {
		it('delegates to getRateTiers query', () => {
			const repo = new SqliteRateTierRepository();
			const tiers = [{ id: 1, name: 'Standard' }] as any;
			mockGetRateTiers.mockReturnValue(tiers);

			const result = repo.getRateTiers();
			expect(mockGetRateTiers).toHaveBeenCalled();
			expect(result).toBe(tiers);
		});
	});

	describe('getRateTier', () => {
		it('delegates to getRateTier query', () => {
			const repo = new SqliteRateTierRepository();
			const tier = { id: 1, name: 'Standard' } as any;
			mockGetRateTier.mockReturnValue(tier);

			expect(repo.getRateTier(1)).toBe(tier);
			expect(mockGetRateTier).toHaveBeenCalledWith(1);
		});

		it('returns null when not found', () => {
			const repo = new SqliteRateTierRepository();
			mockGetRateTier.mockReturnValue(null);
			expect(repo.getRateTier(999)).toBeNull();
		});
	});

	describe('getDefaultTier', () => {
		it('delegates to getDefaultTier query', () => {
			const repo = new SqliteRateTierRepository();
			const tier = { id: 1, name: 'Default', is_default: true } as any;
			mockGetDefaultTier.mockReturnValue(tier);

			expect(repo.getDefaultTier()).toBe(tier);
		});

		it('returns null when no default tier', () => {
			const repo = new SqliteRateTierRepository();
			mockGetDefaultTier.mockReturnValue(null);
			expect(repo.getDefaultTier()).toBeNull();
		});
	});

	describe('createRateTier', () => {
		it('delegates to createRateTier query', async () => {
			const repo = new SqliteRateTierRepository();
			mockCreateRateTier.mockResolvedValue(3);

			const data = { name: 'Premium', is_default: false };
			const id = await repo.createRateTier(data);

			expect(mockCreateRateTier).toHaveBeenCalledWith(data);
			expect(id).toBe(3);
		});
	});

	describe('updateRateTier', () => {
		it('delegates to updateRateTier query', async () => {
			const repo = new SqliteRateTierRepository();
			mockUpdateRateTier.mockResolvedValue(undefined);

			const data = { name: 'Premium Updated', is_default: false };
			await repo.updateRateTier(1, data);

			expect(mockUpdateRateTier).toHaveBeenCalledWith(1, data);
		});
	});

	describe('deleteRateTier', () => {
		it('delegates to deleteRateTier query', async () => {
			const repo = new SqliteRateTierRepository();
			mockDeleteRateTier.mockResolvedValue(undefined);

			await repo.deleteRateTier(2);
			expect(mockDeleteRateTier).toHaveBeenCalledWith(2);
		});

		it('propagates errors', async () => {
			const repo = new SqliteRateTierRepository();
			mockDeleteRateTier.mockRejectedValue(new Error('in use'));

			await expect(repo.deleteRateTier(1)).rejects.toThrow('in use');
		});
	});
});
