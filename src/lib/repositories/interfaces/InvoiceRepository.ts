import type { Invoice, LineItem, AgingBucket, PaginationParams, PaginatedResult } from '$lib/types/index.js';
import type { CreateInvoiceInput, UpdateInvoiceInput, LineItemInput } from './types.js';

export interface InvoiceRepository {
	getInvoices(search?: string, status?: string, pagination?: PaginationParams): Promise<PaginatedResult<Invoice>>;
	getInvoice(id: number): Promise<Invoice | null>;
	getInvoiceLineItems(invoiceId: number): Promise<LineItem[]>;
	getClientInvoices(clientId: number): Promise<Invoice[]>;
	getAgingReport(): Promise<AgingBucket[]>;

	createInvoice(data: CreateInvoiceInput, lineItems: LineItemInput[]): Promise<number>;
	updateInvoice(id: number, data: UpdateInvoiceInput, lineItems: LineItemInput[]): Promise<void>;
	deleteInvoice(id: number): Promise<void>;
	updateInvoiceStatus(id: number, status: string): Promise<void>;
	duplicateInvoice(id: number): Promise<number>;

	bulkDeleteInvoices(ids: number[]): Promise<void>;
	bulkUpdateInvoiceStatus(ids: number[], status: string): Promise<void>;

	markOverdueInvoices(): Promise<number>;
}
