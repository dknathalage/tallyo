import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/payers.js', () => ({
	getPayers: vi.fn(),
	getPayer: vi.fn(),
	createPayer: vi.fn(),
	updatePayer: vi.fn(),
	deletePayer: vi.fn(),
	bulkDeletePayers: vi.fn(),
	getPayerClients: vi.fn(),
	buildPayerSnapshot: vi.fn()
}));

vi.mock('$lib/db/audit.js', () => ({
	computeChanges: vi.fn().mockReturnValue({})
}));

import { SqlitePayerRepository } from './SqlitePayerRepository.js';
import * as queries from '$lib/db/queries/payers.js';
import { computeChanges } from '$lib/db/audit.js';
import type { StorageTransaction } from '$lib/repositories/interfaces/StorageTransaction.js';

const mockGetPayers = vi.mocked(queries.getPayers);
const mockGetPayer = vi.mocked(queries.getPayer);
const mockCreatePayer = vi.mocked(queries.createPayer);
const mockUpdatePayer = vi.mocked(queries.updatePayer);
const mockDeletePayer = vi.mocked(queries.deletePayer);
const mockBulkDeletePayers = vi.mocked(queries.bulkDeletePayers);
const mockGetPayerClients = vi.mocked(queries.getPayerClients);
const mockBuildPayerSnapshot = vi.mocked(queries.buildPayerSnapshot);
const mockComputeChanges = vi.mocked(computeChanges);

function makeMockAudit() {
	return { logAudit: vi.fn(), getEntityHistory: vi.fn() };
}

function makeMockTx(): StorageTransaction {
	return {
		run: vi.fn(async (fn: () => Promise<unknown>) => fn()),
		begin: vi.fn(),
		commit: vi.fn(),
		rollback: vi.fn()
	} as unknown as StorageTransaction;
}

beforeEach(() => {
	vi.clearAllMocks();
	mockComputeChanges.mockReturnValue({});
});

describe('SqlitePayerRepository', () => {
	describe('getPayers', () => {
		it('delegates to getPayers query', () => {
			const repo = new SqlitePayerRepository(makeMockAudit(), makeMockTx());
			const payers = [{ id: 1, name: 'Corp' }] as any;
			mockGetPayers.mockReturnValue(payers);

			const result = repo.getPayers('Corp');
			expect(mockGetPayers).toHaveBeenCalledWith('Corp');
			expect(result).toBe(payers);
		});
	});

	describe('getPayer', () => {
		it('delegates to getPayer query', () => {
			const repo = new SqlitePayerRepository(makeMockAudit(), makeMockTx());
			const payer = { id: 1, name: 'Corp' } as any;
			mockGetPayer.mockReturnValue(payer);

			expect(repo.getPayer(1)).toBe(payer);
			expect(mockGetPayer).toHaveBeenCalledWith(1);
		});

		it('returns null when not found', () => {
			const repo = new SqlitePayerRepository(makeMockAudit(), makeMockTx());
			mockGetPayer.mockReturnValue(null);
			expect(repo.getPayer(999)).toBeNull();
		});
	});

	describe('getPayerClients', () => {
		it('delegates to getPayerClients query', () => {
			const repo = new SqlitePayerRepository(makeMockAudit(), makeMockTx());
			const clients = [{ id: 1, name: 'Alice' }] as any;
			mockGetPayerClients.mockReturnValue(clients);

			expect(repo.getPayerClients(2)).toBe(clients);
			expect(mockGetPayerClients).toHaveBeenCalledWith(2);
		});
	});

	describe('buildPayerSnapshot', () => {
		it('delegates to buildPayerSnapshot query', () => {
			const repo = new SqlitePayerRepository(makeMockAudit(), makeMockTx());
			const snapshot = { name: 'Corp', email: '' } as any;
			mockBuildPayerSnapshot.mockReturnValue(snapshot);

			expect(repo.buildPayerSnapshot(1)).toBe(snapshot);
			expect(mockBuildPayerSnapshot).toHaveBeenCalledWith(1);
		});

		it('accepts null payerId', () => {
			const repo = new SqlitePayerRepository(makeMockAudit(), makeMockTx());
			mockBuildPayerSnapshot.mockReturnValue({} as any);

			repo.buildPayerSnapshot(null);
			expect(mockBuildPayerSnapshot).toHaveBeenCalledWith(null);
		});
	});

	describe('createPayer', () => {
		it('calls createPayer and logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqlitePayerRepository(audit, tx);

			mockCreatePayer.mockResolvedValue(5);

			const data = { name: 'Corp', email: 'corp@example.com' } as any;
			const id = await repo.createPayer(data);

			expect(mockCreatePayer).toHaveBeenCalledWith(data);
			expect(id).toBe(5);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'payer', entity_id: 5, action: 'create' })
			);
		});
	});

	describe('updatePayer', () => {
		it('calls updatePayer and logs audit when changes exist', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqlitePayerRepository(audit, tx);

			mockGetPayer.mockReturnValue({ id: 1, name: 'Corp', email: 'old@corp.com' } as any);
			mockUpdatePayer.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({ email: { old: 'old@corp.com', new: 'new@corp.com' } });

			await repo.updatePayer(1, { name: 'Corp', email: 'new@corp.com' } as any);

			expect(mockUpdatePayer).toHaveBeenCalled();
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'payer', entity_id: 1, action: 'update' })
			);
		});

		it('does not log audit when no changes', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqlitePayerRepository(audit, tx);

			mockGetPayer.mockReturnValue({ id: 1, name: 'Corp' } as any);
			mockUpdatePayer.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({});

			await repo.updatePayer(1, { name: 'Corp' } as any);

			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('does not log audit when payer not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqlitePayerRepository(audit, tx);

			mockGetPayer.mockReturnValue(null);
			mockUpdatePayer.mockResolvedValue(undefined);

			await repo.updatePayer(999, { name: 'Ghost' } as any);

			expect(audit.logAudit).not.toHaveBeenCalled();
		});
	});

	describe('deletePayer', () => {
		it('calls deletePayer and logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqlitePayerRepository(audit, tx);

			mockGetPayer.mockReturnValue({ id: 1, name: 'Corp' } as any);
			mockDeletePayer.mockResolvedValue(undefined);

			await repo.deletePayer(1);

			expect(mockDeletePayer).toHaveBeenCalledWith(1);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'payer', entity_id: 1, action: 'delete', context: 'Corp' })
			);
		});

		it('uses empty string context when payer not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqlitePayerRepository(audit, tx);

			mockGetPayer.mockReturnValue(null);
			mockDeletePayer.mockResolvedValue(undefined);

			await repo.deletePayer(99);

			expect(audit.logAudit).toHaveBeenCalledWith(expect.objectContaining({ context: '' }));
		});
	});

	describe('bulkDeletePayers', () => {
		it('does nothing for empty array', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqlitePayerRepository(audit, tx);

			await repo.bulkDeletePayers([]);

			expect(mockBulkDeletePayers).not.toHaveBeenCalled();
			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('calls bulkDeletePayers and logs audit for each id', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqlitePayerRepository(audit, tx);

			mockGetPayer
				.mockReturnValueOnce({ id: 1, name: 'Corp A' } as any)
				.mockReturnValueOnce({ id: 2, name: 'Corp B' } as any);
			mockBulkDeletePayers.mockResolvedValue(undefined);

			await repo.bulkDeletePayers([1, 2]);

			expect(mockBulkDeletePayers).toHaveBeenCalledWith([1, 2]);
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
		});
	});
});
