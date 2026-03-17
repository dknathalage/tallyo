import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/db/queries/invoices.js', () => ({
	getInvoices: vi.fn(),
	getInvoice: vi.fn(),
	getInvoiceLineItems: vi.fn(),
	getClientInvoices: vi.fn(),
	createInvoice: vi.fn(),
	updateInvoice: vi.fn(),
	deleteInvoice: vi.fn(),
	updateInvoiceStatus: vi.fn(),
	bulkDeleteInvoices: vi.fn(),
	bulkUpdateInvoiceStatus: vi.fn(),
	markOverdueInvoices: vi.fn(),
	duplicateInvoice: vi.fn(),
	getAgingReport: vi.fn()
}));

vi.mock('$lib/db/audit.js', () => ({
	computeChanges: vi.fn().mockReturnValue({})
}));

import { SqliteInvoiceRepository } from './SqliteInvoiceRepository.js';
import * as queries from '$lib/db/queries/invoices.js';
import { computeChanges } from '$lib/db/audit.js';
import type { StorageTransaction } from '$lib/repositories/interfaces/StorageTransaction.js';

const mockGetInvoices = vi.mocked(queries.getInvoices);
const mockGetInvoice = vi.mocked(queries.getInvoice);
const mockGetInvoiceLineItems = vi.mocked(queries.getInvoiceLineItems);
const mockGetClientInvoices = vi.mocked(queries.getClientInvoices);
const mockCreateInvoice = vi.mocked(queries.createInvoice);
const mockUpdateInvoice = vi.mocked(queries.updateInvoice);
const mockDeleteInvoice = vi.mocked(queries.deleteInvoice);
const mockUpdateInvoiceStatus = vi.mocked(queries.updateInvoiceStatus);
const mockBulkDeleteInvoices = vi.mocked(queries.bulkDeleteInvoices);
const mockBulkUpdateInvoiceStatus = vi.mocked(queries.bulkUpdateInvoiceStatus);
const mockMarkOverdueInvoices = vi.mocked(queries.markOverdueInvoices);
const mockDuplicateInvoice = vi.mocked(queries.duplicateInvoice);
const mockGetAgingReport = vi.mocked(queries.getAgingReport);
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

describe('SqliteInvoiceRepository', () => {
	describe('getInvoices', () => {
		it('delegates to getInvoices query', () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);
			const expected = { data: [], total: 0, page: 1, totalPages: 1 } as any;
			mockGetInvoices.mockReturnValue(expected);

			const result = repo.getInvoices('search', 'draft');
			expect(mockGetInvoices).toHaveBeenCalledWith('search', 'draft', undefined);
			expect(result).toBe(expected);
		});

		it('passes pagination param', () => {
			const repo = new SqliteInvoiceRepository(makeMockAudit(), makeMockTx());
			mockGetInvoices.mockReturnValue({ data: [], total: 0, page: 1, totalPages: 1 } as any);
			repo.getInvoices(undefined, undefined, { page: 2, limit: 10 });
			expect(mockGetInvoices).toHaveBeenCalledWith(undefined, undefined, { page: 2, limit: 10 });
		});
	});

	describe('getInvoice', () => {
		it('delegates to getInvoice query', () => {
			const repo = new SqliteInvoiceRepository(makeMockAudit(), makeMockTx());
			const invoice = { id: 1, invoice_number: 'INV-001' } as any;
			mockGetInvoice.mockReturnValue(invoice);

			const result = repo.getInvoice(1);
			expect(mockGetInvoice).toHaveBeenCalledWith(1);
			expect(result).toBe(invoice);
		});

		it('returns null when not found', () => {
			const repo = new SqliteInvoiceRepository(makeMockAudit(), makeMockTx());
			mockGetInvoice.mockReturnValue(null);

			expect(repo.getInvoice(999)).toBeNull();
		});
	});

	describe('getInvoiceLineItems', () => {
		it('delegates to getInvoiceLineItems query', () => {
			const repo = new SqliteInvoiceRepository(makeMockAudit(), makeMockTx());
			const items = [{ id: 1, description: 'X' }] as any;
			mockGetInvoiceLineItems.mockReturnValue(items);

			const result = repo.getInvoiceLineItems(5);
			expect(mockGetInvoiceLineItems).toHaveBeenCalledWith(5);
			expect(result).toBe(items);
		});
	});

	describe('getClientInvoices', () => {
		it('delegates to getClientInvoices query', () => {
			const repo = new SqliteInvoiceRepository(makeMockAudit(), makeMockTx());
			const invoices = [{ id: 1 }] as any;
			mockGetClientInvoices.mockReturnValue(invoices);

			const result = repo.getClientInvoices(3);
			expect(mockGetClientInvoices).toHaveBeenCalledWith(3);
			expect(result).toBe(invoices);
		});
	});

	describe('createInvoice', () => {
		it('runs in transaction, calls createInvoice, logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockCreateInvoice.mockResolvedValue(10);

			const data = { invoice_number: 'INV-001', client_id: 1 } as any;
			const lineItems = [] as any;

			const id = await repo.createInvoice(data, lineItems);

			expect(tx.run).toHaveBeenCalled();
			expect(mockCreateInvoice).toHaveBeenCalledWith(data, lineItems);
			expect(id).toBe(10);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'invoice', entity_id: 10, action: 'create' })
			);
		});
	});

	describe('updateInvoice', () => {
		it('runs in transaction, calls updateInvoice, logs audit when changes exist', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			const old = { id: 1, invoice_number: 'INV-001', status: 'draft' } as any;
			mockGetInvoice.mockReturnValue(old);
			mockUpdateInvoice.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({ status: { old: 'draft', new: 'sent' } });

			const data = { invoice_number: 'INV-001', client_id: 1 } as any;
			await repo.updateInvoice(1, data, []);

			expect(tx.run).toHaveBeenCalled();
			expect(mockUpdateInvoice).toHaveBeenCalledWith(1, data, []);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'invoice', entity_id: 1, action: 'update' })
			);
		});

		it('does not log audit when no changes', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice.mockReturnValue({ id: 1, invoice_number: 'INV-001', status: 'draft' } as any);
			mockUpdateInvoice.mockResolvedValue(undefined);
			mockComputeChanges.mockReturnValue({});

			await repo.updateInvoice(1, { invoice_number: 'INV-001', client_id: 1 } as any, []);

			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('does not log audit when old invoice not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice.mockReturnValue(null);
			mockUpdateInvoice.mockResolvedValue(undefined);

			await repo.updateInvoice(1, { invoice_number: 'INV-001', client_id: 1 } as any, []);

			expect(audit.logAudit).not.toHaveBeenCalled();
		});
	});

	describe('deleteInvoice', () => {
		it('runs in transaction, calls deleteInvoice, logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice.mockReturnValue({ id: 1, invoice_number: 'INV-001' } as any);
			mockDeleteInvoice.mockResolvedValue(undefined);

			await repo.deleteInvoice(1);

			expect(tx.run).toHaveBeenCalled();
			expect(mockDeleteInvoice).toHaveBeenCalledWith(1);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'invoice', entity_id: 1, action: 'delete', context: 'INV-001' })
			);
		});

		it('uses empty string for context when invoice not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice.mockReturnValue(null);
			mockDeleteInvoice.mockResolvedValue(undefined);

			await repo.deleteInvoice(99);

			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ context: '' })
			);
		});
	});

	describe('updateInvoiceStatus', () => {
		it('calls updateInvoiceStatus query and logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice.mockReturnValue({ id: 1, invoice_number: 'INV-001', status: 'sent' } as any);
			mockUpdateInvoiceStatus.mockResolvedValue(undefined);

			await repo.updateInvoiceStatus(1, 'paid');

			expect(mockUpdateInvoiceStatus).toHaveBeenCalledWith(1, 'paid');
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({
					entity_type: 'invoice',
					entity_id: 1,
					action: 'status_change',
					changes: { status: { old: 'sent', new: 'paid' } }
				})
			);
		});
	});

	describe('bulkDeleteInvoices', () => {
		it('does nothing for empty array', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			await repo.bulkDeleteInvoices([]);

			expect(mockBulkDeleteInvoices).not.toHaveBeenCalled();
			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('calls bulkDeleteInvoices and logs audit for each id', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice
				.mockReturnValueOnce({ id: 1, invoice_number: 'INV-001' } as any)
				.mockReturnValueOnce({ id: 2, invoice_number: 'INV-002' } as any);
			mockBulkDeleteInvoices.mockResolvedValue(undefined);

			await repo.bulkDeleteInvoices([1, 2]);

			expect(tx.run).toHaveBeenCalled();
			expect(mockBulkDeleteInvoices).toHaveBeenCalledWith([1, 2]);
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
		});
	});

	describe('bulkUpdateInvoiceStatus', () => {
		it('does nothing for empty array', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			await repo.bulkUpdateInvoiceStatus([], 'paid');

			expect(mockBulkUpdateInvoiceStatus).not.toHaveBeenCalled();
			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('calls bulkUpdateInvoiceStatus and logs audit for each id', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice
				.mockReturnValueOnce({ id: 1, invoice_number: 'INV-001', status: 'sent' } as any)
				.mockReturnValueOnce({ id: 2, invoice_number: 'INV-002', status: 'draft' } as any);
			mockBulkUpdateInvoiceStatus.mockResolvedValue(undefined);

			await repo.bulkUpdateInvoiceStatus([1, 2], 'paid');

			expect(mockBulkUpdateInvoiceStatus).toHaveBeenCalledWith([1, 2], 'paid');
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
		});
	});

	describe('markOverdueInvoices', () => {
		it('returns 0 when no invoices updated', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockMarkOverdueInvoices.mockResolvedValue([]);

			const count = await repo.markOverdueInvoices();
			expect(count).toBe(0);
			expect(audit.logAudit).not.toHaveBeenCalled();
		});

		it('logs audit for each updated invoice', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockMarkOverdueInvoices.mockResolvedValue([
				{ id: 1, invoice_number: 'INV-001' },
				{ id: 2, invoice_number: 'INV-002' }
			]);

			const count = await repo.markOverdueInvoices();

			expect(count).toBe(2);
			expect(audit.logAudit).toHaveBeenCalledTimes(2);
		});
	});

	describe('duplicateInvoice', () => {
		it('runs in transaction, calls duplicateInvoice, logs audit', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice.mockReturnValue({ id: 1, invoice_number: 'INV-001' } as any);
			mockDuplicateInvoice.mockResolvedValue(5);

			const newId = await repo.duplicateInvoice(1);

			expect(tx.run).toHaveBeenCalled();
			expect(mockDuplicateInvoice).toHaveBeenCalledWith(1);
			expect(newId).toBe(5);
			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ entity_type: 'invoice', entity_id: 5, action: 'create' })
			);
		});

		it('uses fallback context when original not found', async () => {
			const audit = makeMockAudit();
			const tx = makeMockTx();
			const repo = new SqliteInvoiceRepository(audit, tx);

			mockGetInvoice.mockReturnValue(null);
			mockDuplicateInvoice.mockResolvedValue(5);

			await repo.duplicateInvoice(999);

			expect(audit.logAudit).toHaveBeenCalledWith(
				expect.objectContaining({ context: '(duplicated from invoice 999)' })
			);
		});
	});

	describe('getAgingReport', () => {
		it('delegates to getAgingReport query', () => {
			const repo = new SqliteInvoiceRepository(makeMockAudit(), makeMockTx());
			const buckets = [{ label: 'Current', total: 0, invoices: [] }] as any;
			mockGetAgingReport.mockReturnValue(buckets);

			const result = repo.getAgingReport();
			expect(mockGetAgingReport).toHaveBeenCalled();
			expect(result).toBe(buckets);
		});
	});
});
