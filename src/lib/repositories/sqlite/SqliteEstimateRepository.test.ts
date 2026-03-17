import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/estimates.js', () => ({
	getEstimates: vi.fn(),
	getEstimate: vi.fn(),
	getEstimateLineItems: vi.fn(),
	getClientEstimates: vi.fn(),
	createEstimate: vi.fn(),
	updateEstimate: vi.fn(),
	deleteEstimate: vi.fn(),
	updateEstimateStatus: vi.fn(),
	bulkDeleteEstimates: vi.fn(),
	bulkUpdateEstimateStatus: vi.fn(),
	convertEstimateToInvoice: vi.fn(),
	duplicateEstimate: vi.fn()
}));

vi.mock('$lib/db/audit.js', () => ({
	computeChanges: vi.fn().mockReturnValue({})
}));

import { SqliteEstimateRepository } from './SqliteEstimateRepository.js';
import * as queries from '$lib/db/queries/estimates.js';
import { computeChanges } from '$lib/db/audit.js';
import type { StorageTransaction } from '$lib/repositories/interfaces/StorageTransaction.js';

const mockGetEstimates = vi.mocked(queries.getEstimates);
const mockGetEstimate = vi.mocked(queries.getEstimate);
const mockGetEstimateLineItems = vi.mocked(queries.getEstimateLineItems);
const mockGetClientEstimates = vi.mocked(queries.getClientEstimates);
const mockCreateEstimate = vi.mocked(queries.createEstimate);
const mockUpdateEstimate = vi.mocked(queries.updateEstimate);
const mockDeleteEstimate = vi.mocked(queries.deleteEstimate);
const mockUpdateEstimateStatus = vi.mocked(queries.updateEstimateStatus);
const mockBulkDeleteEstimates = vi.mocked(queries.bulkDeleteEstimates);
const mockBulkUpdateEstimateStatus = vi.mocked(queries.bulkUpdateEstimateStatus);
const mockConvertEstimateToInvoice = vi.mocked(queries.convertEstimateToInvoice);
const mockDuplicateEstimate = vi.mocked(queries.duplicateEstimate);
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

describe('SqliteEstimateRepository', () => {
	describe('getEstimates', () => {
		it('delegates to getEstimates query', () => {
			const repo = new SqliteEstimateRepository(makeMockAudit(), makeMockTx());
			const expected = { data: [], total: 0, page: 1, totalPages: 1 } as any;
			mockGetEstimates.mockReturnValue(expected);

			const result = repo.getEstimates('search', 'draft');
			expect(mockGetEstimates).toHaveBeenCalledWith('search', 'draft', undefined);
			expect(result).toBe(expected);
		});
	});

	describe('getEstimate', () => {
		it('delegates to getEstimate query', () => {
			const repo = new SqliteEstimateRepository(makeMockAudit(), makeMockTx());
			const estimate = { id: 1, estimate_number: 'EST-001' } as any;
			mockGetEstimate.mockReturnValue(estimate);

			expect(repo.getEstimate(1)).toBe(estimate);
			expect(mockGetEstimate).toHaveBeenCalledWith(1);
		});

		it('returns null when not found', () => {
			const repo = new SqliteEstimateRepository(makeMockAudit(), makeMockTx());
			mockGetEstimate.mockReturnValue(null);
			expect(repo.getEstimate(999)).toBeNull();
		});
	});

	describe('getEstimateLineItems', () => {
		it('delegates to getEstimateLineItems query', () => {
			const repo = new SqliteEstimateRepository(makeMockAudit(), makeMockTx());
			const items = [{ id: 1 }] as any;
			mockGetEstimateLineItems.mockReturnValue(items);

			expect(repo.getEstimateLineItems(5)).toBe(items);
			expect(mockGetEstimateLineItems).toHaveBeenCalledWith(5);
		});
	});

	describe('getClientEstimates', () => {
		it('delegates to getClientEstimates query', () => {
			const repo = new SqliteEstimateRepository(makeMockAudit(), makeMockTx());
			const estimates = [{ id: 1 }] as any;
			mockGetClientEstimates.mockReturnValue(estimates);

			expect(repo.getClientEstimates(3)).toBe(estimates);
			expect(mockGetClientEstimates).toHaveBeenCalledWith(3);
		});
	});

	describe('createEstimate', () => {
		it('runs in transaction, calls createEstimate, logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockCreateEstimate.mockResolvedValue(10);

			const data = { estimate_number: 'EST-001', client_id: 1 } as any;
			const id = await repo.createEstimate(data, []);

			expect(tx.run).toHaveBeenCalled();
			expect(mockCreateEstimate).toHaveBeenCalledWith(data, []);
			expect(id).toBe(10);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'estimate', entity_id: 10, action: 'create' })
			);
		});
	});

	describe('updateEstimate', () => {
		it('runs in transaction, calls updateEstimate, logs audit when changes exist', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockGetEstimate.mockReturnValue({ id: 1, estimate_number: 'EST-001', status: 'draft' } as any);
			mockUpdateEstimate.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({ status: { old: 'draft', new: 'sent' } });

			await repo.updateEstimate(1, { estimate_number: 'EST-001', client_id: 1 } as any, []);

			expect(tx.run).toHaveBeenCalled();
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'estimate', entity_id: 1, action: 'update' })
			);
		});

		it('does not log audit when no changes', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockGetEstimate.mockReturnValue({ id: 1, estimate_number: 'EST-001' } as any);
			mockUpdateEstimate.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({});

			await repo.updateEstimate(1, { estimate_number: 'EST-001', client_id: 1 } as any, []);

			expect(audit.logAudit).not.toHaveBeenCalled();
		});
	});

	describe('deleteEstimate', () => {
		it('runs in transaction, calls deleteEstimate, logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockGetEstimate.mockReturnValue({ id: 1, estimate_number: 'EST-001' } as any);
			mockDeleteEstimate.mockResolvedValue(undefined);

			await repo.deleteEstimate(1);

			expect(tx.run).toHaveBeenCalled();
			expect(mockDeleteEstimate).toHaveBeenCalledWith(1);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'estimate', entity_id: 1, action: 'delete', context: 'EST-001' })
			);
		});

		it('uses empty string context when estimate not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockGetEstimate.mockReturnValue(null);
			mockDeleteEstimate.mockResolvedValue(undefined);

			await repo.deleteEstimate(99);

			expect(audit.logAudit).toHaveBeenCalledWith(expect.objectContaining({ context: '' }));
		});
	});

	describe('updateEstimateStatus', () => {
		it('calls updateEstimateStatus and logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockGetEstimate.mockReturnValue({ id: 1, estimate_number: 'EST-001', status: 'draft' } as any);
			mockUpdateEstimateStatus.mockResolvedValue(undefined);

			await repo.updateEstimateStatus(1, 'sent');

			expect(mockUpdateEstimateStatus).toHaveBeenCalledWith(1, 'sent');
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({
					entity_type: 'estimate',
					entity_id: 1,
					action: 'status_change',
					changes: { status: { old: 'draft', new: 'sent' } }
				})
			);
		});
	});

	describe('bulkDeleteEstimates', () => {
		it('does nothing for empty array', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			await repo.bulkDeleteEstimates([]);

			expect(mockBulkDeleteEstimates).not.toHaveBeenCalled();
			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('calls bulkDeleteEstimates and logs audit for each id', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockGetEstimate
				.mockReturnValueOnce({ id: 1, estimate_number: 'EST-001' } as any)
				.mockReturnValueOnce({ id: 2, estimate_number: 'EST-002' } as any);
			mockBulkDeleteEstimates.mockResolvedValue(undefined);

			await repo.bulkDeleteEstimates([1, 2]);

			expect(mockBulkDeleteEstimates).toHaveBeenCalledWith([1, 2]);
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
		});
	});

	describe('bulkUpdateEstimateStatus', () => {
		it('does nothing for empty array', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			await repo.bulkUpdateEstimateStatus([], 'sent');

			expect(mockBulkUpdateEstimateStatus).not.toHaveBeenCalled();
		});

		it('calls bulkUpdateEstimateStatus and logs audit for each id', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockGetEstimate
				.mockReturnValueOnce({ id: 1, estimate_number: 'EST-001', status: 'draft' } as any)
				.mockReturnValueOnce({ id: 2, estimate_number: 'EST-002', status: 'draft' } as any);
			mockBulkUpdateEstimateStatus.mockResolvedValue(undefined);

			await repo.bulkUpdateEstimateStatus([1, 2], 'sent');

			expect(mockBulkUpdateEstimateStatus).toHaveBeenCalledWith([1, 2], 'sent');
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
		});
	});

	describe('convertEstimateToInvoice', () => {
		it('runs in transaction, logs two audit entries', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockConvertEstimateToInvoice.mockResolvedValue({
				invoiceId: 10,
				invoiceNumber: 'INV-001',
				estimateNumber: 'EST-001'
			});

			const invoiceId = await repo.convertEstimateToInvoice(5);

			expect(tx.run).toHaveBeenCalled();
			expect(invoiceId).toBe(10);
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'estimate', action: 'convert' })
			);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'invoice', action: 'create' })
			);
		});
	});

	describe('duplicateEstimate', () => {
		it('runs in transaction, calls duplicateEstimate, logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteEstimateRepository(audit, tx);

			mockDuplicateEstimate.mockResolvedValue({
				newId: 7,
				newNumber: 'EST-002',
				originalNumber: 'EST-001'
			});

			const newId = await repo.duplicateEstimate(1);

			expect(tx.run).toHaveBeenCalled();
			expect(newId).toBe(7);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'estimate', entity_id: 7, action: 'create' })
			);
		});
	});
});
