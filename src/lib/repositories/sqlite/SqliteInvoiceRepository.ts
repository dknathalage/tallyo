import {
	getInvoices,
	getInvoice,
	getInvoiceLineItems,
	getClientInvoices,
	createInvoice,
	updateInvoice,
	deleteInvoice,
	updateInvoiceStatus,
	bulkDeleteInvoices,
	bulkUpdateInvoiceStatus,
	markOverdueInvoices,
	duplicateInvoice,
	getAgingReport
} from '$lib/db/queries/invoices.js';
import type { InvoiceRepository } from '../interfaces/InvoiceRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { StorageTransaction } from '../interfaces/StorageTransaction.js';
import type { CreateInvoiceInput, UpdateInvoiceInput, LineItemInput } from '../interfaces/types.js';
import type { Invoice, LineItem, AgingBucket } from '$lib/types/index.js';

export class SqliteInvoiceRepository implements InvoiceRepository {
	constructor(
		private readonly _audit?: AuditRepository,
		private readonly _tx?: StorageTransaction
	) {}

	getInvoices(search?: string, status?: string): Invoice[] {
		return getInvoices(search, status);
	}

	getInvoice(id: number): Invoice | null {
		return getInvoice(id);
	}

	getInvoiceLineItems(invoiceId: number): LineItem[] {
		return getInvoiceLineItems(invoiceId);
	}

	getClientInvoices(clientId: number): Invoice[] {
		return getClientInvoices(clientId);
	}

	createInvoice(data: CreateInvoiceInput, lineItems: LineItemInput[]): Promise<number> {
		return createInvoice(data, lineItems);
	}

	updateInvoice(id: number, data: UpdateInvoiceInput, lineItems: LineItemInput[]): Promise<void> {
		return updateInvoice(id, data, lineItems);
	}

	deleteInvoice(id: number): Promise<void> {
		return deleteInvoice(id);
	}

	updateInvoiceStatus(id: number, status: string): Promise<void> {
		return updateInvoiceStatus(id, status);
	}

	bulkDeleteInvoices(ids: number[]): Promise<void> {
		return bulkDeleteInvoices(ids);
	}

	bulkUpdateInvoiceStatus(ids: number[], status: string): Promise<void> {
		return bulkUpdateInvoiceStatus(ids, status);
	}

	markOverdueInvoices(): Promise<number> {
		return markOverdueInvoices();
	}

	duplicateInvoice(id: number): Promise<number> {
		return duplicateInvoice(id);
	}

	getAgingReport(): AgingBucket[] {
		return getAgingReport();
	}
}
