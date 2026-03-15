import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/business-profile.js', () => ({
	getBusinessProfile: vi.fn(),
	saveBusinessProfile: vi.fn(),
	buildBusinessSnapshot: vi.fn()
}));

import { SqliteBusinessProfileRepository } from './SqliteBusinessProfileRepository.js';
import * as queries from '$lib/db/queries/business-profile.js';

const mockGetBusinessProfile = vi.mocked(queries.getBusinessProfile);
const mockSaveBusinessProfile = vi.mocked(queries.saveBusinessProfile);
const mockBuildBusinessSnapshot = vi.mocked(queries.buildBusinessSnapshot);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('SqliteBusinessProfileRepository', () => {
	describe('getBusinessProfile', () => {
		it('delegates to getBusinessProfile query', () => {
			const repo = new SqliteBusinessProfileRepository();
			const profile = { id: 1, name: 'My Business' } as any;
			mockGetBusinessProfile.mockReturnValue(profile);

			const result = repo.getBusinessProfile();
			expect(mockGetBusinessProfile).toHaveBeenCalled();
			expect(result).toBe(profile);
		});

		it('returns null when no profile', () => {
			const repo = new SqliteBusinessProfileRepository();
			mockGetBusinessProfile.mockReturnValue(null);

			expect(repo.getBusinessProfile()).toBeNull();
		});
	});

	describe('buildBusinessSnapshot', () => {
		it('delegates to buildBusinessSnapshot query', () => {
			const repo = new SqliteBusinessProfileRepository();
			const snapshot = { name: 'My Business', email: 'biz@example.com' } as any;
			mockBuildBusinessSnapshot.mockReturnValue(snapshot);

			const result = repo.buildBusinessSnapshot();
			expect(mockBuildBusinessSnapshot).toHaveBeenCalled();
			expect(result).toBe(snapshot);
		});
	});

	describe('saveBusinessProfile', () => {
		it('delegates to saveBusinessProfile query', async () => {
			const repo = new SqliteBusinessProfileRepository();
			mockSaveBusinessProfile.mockResolvedValue(undefined);

			const data = { name: 'Updated Business', email: 'updated@biz.com' } as any;
			await repo.saveBusinessProfile(data);

			expect(mockSaveBusinessProfile).toHaveBeenCalledWith(data);
		});

		it('propagates errors from saveBusinessProfile', async () => {
			const repo = new SqliteBusinessProfileRepository();
			mockSaveBusinessProfile.mockRejectedValue(new Error('save failed'));

			await expect(repo.saveBusinessProfile({} as any)).rejects.toThrow('save failed');
		});
	});
});
