import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/audit.js', () => ({
	getEntityHistory: vi.fn()
}));

vi.mock('$lib/db/audit.js', () => ({
	logAudit: vi.fn(),
	computeChanges: vi.fn()
}));

import { SqliteAuditRepository } from './SqliteAuditRepository.js';
import { getEntityHistory } from '$lib/db/queries/audit.js';
import { logAudit } from '$lib/db/audit.js';

const mockGetEntityHistory = vi.mocked(getEntityHistory);
const mockLogAudit = vi.mocked(logAudit);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('SqliteAuditRepository', () => {
	describe('getEntityHistory', () => {
		it('delegates to getEntityHistory query', () => {
			const repo = new SqliteAuditRepository();
			const entries = [{ id: 1, entity_type: 'invoice', entity_id: 1, action: 'create' }] as any;
			mockGetEntityHistory.mockReturnValue(entries);

			const result = repo.getEntityHistory('invoice', 1);
			expect(mockGetEntityHistory).toHaveBeenCalledWith('invoice', 1);
			expect(result).toBe(entries);
		});

		it('returns empty array when no history', () => {
			const repo = new SqliteAuditRepository();
			mockGetEntityHistory.mockReturnValue([]);

			const result = repo.getEntityHistory('client', 999);
			expect(result).toEqual([]);
		});
	});

	describe('logAudit', () => {
		it('delegates to logAudit function', () => {
			const repo = new SqliteAuditRepository();
			const params = { entity_type: 'invoice', entity_id: 1, action: 'create' } as any;

			repo.logAudit(params);

			expect(mockLogAudit).toHaveBeenCalledWith(params);
		});

		it('passes all audit params', () => {
			const repo = new SqliteAuditRepository();
			const params = {
				entity_type: 'client',
				entity_id: 5,
				action: 'update',
				changes: { name: { old: 'Alice', new: 'Alicia' } },
				context: 'name change',
				batch_id: 'abc-123'
			} as any;

			repo.logAudit(params);

			expect(mockLogAudit).toHaveBeenCalledWith(params);
		});
	});
});
