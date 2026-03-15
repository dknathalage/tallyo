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
import type { StorageTransaction } from '../interfaces/StorageTransaction.js';
import type { CreateInvoiceInput, UpdateInvoiceInput, LineItemInput } from '../interfaces/types.js';
import type { Invoice, LineItem, AgingBucket } from '$lib/types/index.js';

export class SqliteInvoiceRepository implements InvoiceRepository {
	constructor(
		private readonly _audit: AuditRepository,
		private readonly _tx: StorageTransaction
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

	async createInvoice(data: CreateInvoiceInput, lineItems: LineItemInput[]): Promise<number> {
		const id = await this._tx.run(async () => {
			return await createInvoice(data, lineItems);
		});
		this._audit.logAudit({
			entity_type: 'invoice',
			entity_id: id,
			action: 'create',
			context: data.invoice_number
		});
		return id;
	}

	async updateInvoice(id: number, data: UpdateInvoiceInput, lineItems: LineItemInput[]): Promise<void> {
		const oldInvoice = getInvoice(id);
		await this._tx.run(async () => {
			await updateInvoice(id, data, lineItems);
		});
		if (oldInvoice) {
			const changes = computeChanges(
				oldInvoice as unknown as Record<string, unknown>,
				{ ...data, notes: data.notes ?? '', status: data.status ?? 'draft', currency_code: data.currency_code ?? 'USD' },
				['invoice_number', 'client_id', 'date', 'due_date', 'subtotal', 'tax_rate', 'total', 'notes', 'status', 'currency_code']
			);
			if (Object.keys(changes).length > 0) {
				this._audit.logAudit({ entity_type: 'invoice', entity_id: id, action: 'update', changes, context: data.invoice_number });
			}
		}
	}

	async deleteInvoice(id: number): Promise<void> {
		const invoice = getInvoice(id);
		await this._tx.run(async () => {
			await deleteInvoice(id);
		});
		this._audit.logAudit({ entity_type: 'invoice', entity_id: id, action: 'delete', context: invoice?.invoice_number ?? '' });
	}

	async updateInvoiceStatus(id: number, status: string): Promise<void> {
		const oldInvoice = getInvoice(id);
		await updateInvoiceStatus(id, status);
		this._audit.logAudit({
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
		const invoices = ids.map((id) => getInvoice(id));
		await this._tx.run(async () => {
			await bulkDeleteInvoices(ids);
		});
		for (let i = 0; i < ids.length; i++) {
			this._audit.logAudit({ entity_type: 'invoice', entity_id: ids[i], action: 'delete', context: invoices[i]?.invoice_number ?? '', batch_id });
		}
	}

	async bulkUpdateInvoiceStatus(ids: number[], status: string): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const invoices = ids.map((id) => getInvoice(id));
		await bulkUpdateInvoiceStatus(ids, status);
		for (let i = 0; i < ids.length; i++) {
			this._audit.logAudit({
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
			this._audit.logAudit({
				entity_type: 'invoice',
				entity_id: inv.id,
				action: 'status_change',
				changes: { status: { old: 'sent', new: 'overdue' } },
				context: inv.invoice_number
			});
		}
		if (updated.length > 0) {
		}
		return updated.length;
	}

	async duplicateInvoice(id: number): Promise<number> {
		const original = getInvoice(id);
		const newId = await this._tx.run(async () => {
			return await duplicateInvoice(id);
		});
		this._audit.logAudit({
			entity_type: 'invoice',
			entity_id: newId,
			action: 'create',
			context: original ? `(duplicated from ${original.invoice_number})` : `(duplicated from invoice ${id})`
		});
		return newId;
	}

	getAgingReport(): AgingBucket[] {
		return getAgingReport();
	}
}
