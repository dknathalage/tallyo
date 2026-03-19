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

import { computeChanges } from '$lib/db/audit.js';
import type { InvoiceRepository } from '../interfaces/InvoiceRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { CreateInvoiceInput, UpdateInvoiceInput, LineItemInput } from '../interfaces/types.js';
import type { Invoice, LineItem, AgingBucket, PaginationParams, PaginatedResult } from '$lib/types/index.js';

export class PgInvoiceRepository implements InvoiceRepository {
	constructor(private readonly _audit: AuditRepository) {}

	async getInvoices(search?: string, status?: string, pagination?: PaginationParams): Promise<PaginatedResult<Invoice>> {
		return await getInvoices(search, status, pagination);
	}

	async getInvoice(id: number): Promise<Invoice | null> {
		return await getInvoice(id);
	}

	async getInvoiceLineItems(invoiceId: number): Promise<LineItem[]> {
		return await getInvoiceLineItems(invoiceId);
	}

	async getClientInvoices(clientId: number): Promise<Invoice[]> {
		return await getClientInvoices(clientId);
	}

	async createInvoice(data: CreateInvoiceInput, lineItems: LineItemInput[]): Promise<number> {
		const id = await createInvoice(data, lineItems);
		await this._audit.logAudit({
			entity_type: 'invoice',
			entity_id: id,
			action: 'create',
			context: data.invoice_number
		});
		return id;
	}

	async updateInvoice(id: number, data: UpdateInvoiceInput, lineItems: LineItemInput[]): Promise<void> {
		const oldInvoice = await getInvoice(id);
		await updateInvoice(id, data, lineItems);
		if (oldInvoice) {
			const changes = computeChanges(
				oldInvoice as unknown as Record<string, unknown>,
				{ ...data, notes: data.notes ?? '', status: data.status ?? 'draft', currency_code: data.currency_code ?? 'USD' },
				['invoice_number', 'client_id', 'date', 'due_date', 'subtotal', 'tax_rate', 'total', 'notes', 'status', 'currency_code']
			);
			if (Object.keys(changes).length > 0) {
				await this._audit.logAudit({ entity_type: 'invoice', entity_id: id, action: 'update', changes, context: data.invoice_number });
			}
		}
	}

	async deleteInvoice(id: number): Promise<void> {
		const invoice = await getInvoice(id);
		await deleteInvoice(id);
		await this._audit.logAudit({ entity_type: 'invoice', entity_id: id, action: 'delete', context: invoice?.invoice_number ?? '' });
	}

	async updateInvoiceStatus(id: number, status: string): Promise<void> {
		const oldInvoice = await getInvoice(id);
		await updateInvoiceStatus(id, status);
		await this._audit.logAudit({
			entity_type: 'invoice',
			entity_id: id,
			action: 'status_change',
			changes: { status: { old: oldInvoice?.status ?? '', new: status } },
			context: oldInvoice?.invoice_number ?? ''
		});
	}

	async bulkDeleteInvoices(ids: number[]): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const invoices = await Promise.all(ids.map((id) => getInvoice(id)));
		await bulkDeleteInvoices(ids);
		for (let i = 0; i < ids.length; i++) {
			await this._audit.logAudit({ entity_type: 'invoice', entity_id: ids[i], action: 'delete', context: invoices[i]?.invoice_number ?? '', batch_id });
		}
	}

	async bulkUpdateInvoiceStatus(ids: number[], status: string): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const invoices = await Promise.all(ids.map((id) => getInvoice(id)));
		await bulkUpdateInvoiceStatus(ids, status);
		for (let i = 0; i < ids.length; i++) {
			await this._audit.logAudit({
				entity_type: 'invoice',
				entity_id: ids[i],
				action: 'status_change',
				changes: { status: { old: invoices[i]?.status ?? '', new: status } },
				context: invoices[i]?.invoice_number ?? '',
				batch_id
			});
		}
	}

	async markOverdueInvoices(): Promise<number> {
		const updated = await markOverdueInvoices();
		for (const inv of updated) {
			await this._audit.logAudit({
				entity_type: 'invoice',
				entity_id: inv.id,
				action: 'status_change',
				changes: { status: { old: 'sent', new: 'overdue' } },
				context: inv.invoice_number
			});
		}
		return updated.length;
	}

	async duplicateInvoice(id: number): Promise<number> {
		const original = await getInvoice(id);
		const newId = await duplicateInvoice(id);
		await this._audit.logAudit({
			entity_type: 'invoice',
			entity_id: newId,
			action: 'create',
			context: original ? `(duplicated from ${original.invoice_number})` : `(duplicated from invoice ${id})`
		});
		return newId;
	}

	async getAgingReport(): Promise<AgingBucket[]> {
		return await getAgingReport();
	}
}
