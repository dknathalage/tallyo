import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/clients.js', () => ({
	getClients: vi.fn(),
	getClient: vi.fn(),
	createClient: vi.fn(),
	updateClient: vi.fn(),
	deleteClient: vi.fn(),
	bulkDeleteClients: vi.fn(),
	buildClientSnapshot: vi.fn(),
	getClientRevenueSummary: vi.fn()
}));

vi.mock('$lib/db/audit.js', () => ({
	computeChanges: vi.fn().mockReturnValue({})
}));

import { SqliteClientRepository } from './SqliteClientRepository.js';
import * as queries from '$lib/db/queries/clients.js';
import { computeChanges } from '$lib/db/audit.js';
import type { StorageTransaction } from '$lib/repositories/interfaces/StorageTransaction.js';

const mockGetClients = vi.mocked(queries.getClients);
const mockGetClient = vi.mocked(queries.getClient);
const mockCreateClient = vi.mocked(queries.createClient);
const mockUpdateClient = vi.mocked(queries.updateClient);
const mockDeleteClient = vi.mocked(queries.deleteClient);
const mockBulkDeleteClients = vi.mocked(queries.bulkDeleteClients);
const mockBuildClientSnapshot = vi.mocked(queries.buildClientSnapshot);
const mockGetClientRevenueSummary = vi.mocked(queries.getClientRevenueSummary);
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

describe('SqliteClientRepository', () => {
	describe('getClients', () => {
		it('delegates to getClients query', () => {
			const repo = new SqliteClientRepository(makeMockAudit(), makeMockTx());
			const expected = { data: [], total: 0, page: 1, totalPages: 1 } as any;
			mockGetClients.mockReturnValue(expected);

			const result = repo.getClients('alice');
			expect(mockGetClients).toHaveBeenCalledWith('alice', undefined);
			expect(result).toBe(expected);
		});

		it('passes pagination param', () => {
			const repo = new SqliteClientRepository(makeMockAudit(), makeMockTx());
			mockGetClients.mockReturnValue({ data: [], total: 0, page: 1, totalPages: 1 } as any);
			repo.getClients(undefined, { page: 2, limit: 20 });
			expect(mockGetClients).toHaveBeenCalledWith(undefined, { page: 2, limit: 20 });
		});
	});

	describe('getClient', () => {
		it('delegates to getClient query', () => {
			const repo = new SqliteClientRepository(makeMockAudit(), makeMockTx());
			const client = { id: 1, name: 'Alice' } as any;
			mockGetClient.mockReturnValue(client);

			expect(repo.getClient(1)).toBe(client);
			expect(mockGetClient).toHaveBeenCalledWith(1);
		});

		it('returns null when not found', () => {
			const repo = new SqliteClientRepository(makeMockAudit(), makeMockTx());
			mockGetClient.mockReturnValue(null);
			expect(repo.getClient(999)).toBeNull();
		});
	});

	describe('buildClientSnapshot', () => {
		it('delegates to buildClientSnapshot query', () => {
			const repo = new SqliteClientRepository(makeMockAudit(), makeMockTx());
			const snapshot = { name: 'Alice', email: '' } as any;
			mockBuildClientSnapshot.mockReturnValue(snapshot);

			expect(repo.buildClientSnapshot(1)).toBe(snapshot);
			expect(mockBuildClientSnapshot).toHaveBeenCalledWith(1);
		});
	});

	describe('getClientRevenueSummary', () => {
		it('delegates to getClientRevenueSummary query', () => {
			const repo = new SqliteClientRepository(makeMockAudit(), makeMockTx());
			const summary = { total_invoiced: 1000 } as any;
			mockGetClientRevenueSummary.mockReturnValue(summary);

			expect(repo.getClientRevenueSummary(1)).toBe(summary);
			expect(mockGetClientRevenueSummary).toHaveBeenCalledWith(1);
		});
	});

	describe('createClient', () => {
		it('calls createClient and logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteClientRepository(audit, tx);

			mockCreateClient.mockResolvedValue(5);

			const id = await repo.createClient({ name: 'Bob', email: 'bob@test.com' });

			expect(mockCreateClient).toHaveBeenCalledWith({ name: 'Bob', email: 'bob@test.com' });
			expect(id).toBe(5);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({
					entity_type: 'client',
					entity_id: 5,
					action: 'create',
					changes: expect.objectContaining({ name: expect.any(Object) })
				})
			);
		});
	});

	describe('updateClient', () => {
		it('calls updateClient and logs audit when changes exist', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteClientRepository(audit, tx);

			mockGetClient.mockReturnValue({ id: 1, name: 'Alice', email: 'alice@old.com' } as any);
			mockUpdateClient.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({ email: { old: 'alice@old.com', new: 'alice@new.com' } });

			await repo.updateClient(1, { name: 'Alice', email: 'alice@new.com' });

			expect(mockUpdateClient).toHaveBeenCalledWith(1, { name: 'Alice', email: 'alice@new.com' });
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'client', entity_id: 1, action: 'update' })
			);
		});

		it('does not log audit when no changes', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteClientRepository(audit, tx);

			mockGetClient.mockReturnValue({ id: 1, name: 'Alice' } as any);
			mockUpdateClient.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({});

			await repo.updateClient(1, { name: 'Alice' });

			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('does not log audit when client not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteClientRepository(audit, tx);

			mockGetClient.mockReturnValue(null);
			mockUpdateClient.mockResolvedValue(undefined);

			await repo.updateClient(999, { name: 'Ghost' });

			expect(audit.logAudit).not.toHaveBeenCalled();
		});
	});

	describe('deleteClient', () => {
		it('runs in transaction, calls deleteClient, logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteClientRepository(audit, tx);

			mockGetClient.mockReturnValue({ id: 1, name: 'Alice' } as any);
			mockDeleteClient.mockResolvedValue(undefined);

			await repo.deleteClient(1);

			expect(tx.run).toHaveBeenCalled();
			expect(mockDeleteClient).toHaveBeenCalledWith(1);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'client', entity_id: 1, action: 'delete', context: 'Alice' })
			);
		});

		it('uses empty string context when client not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteClientRepository(audit, tx);

			mockGetClient.mockReturnValue(null);
			mockDeleteClient.mockResolvedValue(undefined);

			await repo.deleteClient(99);

			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ context: '' })
			);
		});
	});

	describe('bulkDeleteClients', () => {
		it('does nothing for empty array', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteClientRepository(audit, tx);

			await repo.bulkDeleteClients([]);

			expect(mockBulkDeleteClients).not.toHaveBeenCalled();
			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('calls bulkDeleteClients and logs audit for each id', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteClientRepository(audit, tx);

			mockGetClient
				.mockReturnValueOnce({ id: 1, name: 'Alice' } as any)
				.mockReturnValueOnce({ id: 2, name: 'Bob' } as any);
			mockBulkDeleteClients.mockResolvedValue(undefined);

			await repo.bulkDeleteClients([1, 2]);

			expect(tx.run).toHaveBeenCalled();
			expect(mockBulkDeleteClients).toHaveBeenCalledWith([1, 2]);
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
		});
	});
});
