import {
	getEstimates,
	getEstimate,
	getEstimateLineItems,
	getClientEstimates,
	createEstimate,
	updateEstimate,
	deleteEstimate,
	updateEstimateStatus,
	bulkDeleteEstimates,
	bulkUpdateEstimateStatus,
	convertEstimateToInvoice,
	duplicateEstimate
} from '$lib/db/queries/estimates.js';

import { computeChanges } from '$lib/db/audit.js';
import type { EstimateRepository } from '../interfaces/EstimateRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { CreateEstimateInput, UpdateEstimateInput, LineItemInput } from '../interfaces/types.js';
import type { Estimate, EstimateLineItem, PaginationParams, PaginatedResult } from '$lib/types/index.js';

export class SqliteEstimateRepository implements EstimateRepository {
	constructor(private readonly _audit: AuditRepository) {}

	async getEstimates(search?: string, status?: string, pagination?: PaginationParams): Promise<PaginatedResult<Estimate>> {
		return await getEstimates(search, status, pagination);
	}

	async getEstimate(id: number): Promise<Estimate | null> {
		return await getEstimate(id);
	}

	async getEstimateLineItems(estimateId: number): Promise<EstimateLineItem[]> {
		return await getEstimateLineItems(estimateId);
	}

	async getClientEstimates(clientId: number): Promise<Estimate[]> {
		return await getClientEstimates(clientId);
	}

	async createEstimate(data: CreateEstimateInput, lineItems: LineItemInput[]): Promise<number> {
		const id = await createEstimate(data, lineItems);
		await this._audit.logAudit({
			entity_type: 'estimate',
			entity_id: id,
			action: 'create',
			context: data.estimate_number
		});
		return id;
	}

	async updateEstimate(id: number, data: UpdateEstimateInput, lineItems: LineItemInput[]): Promise<void> {
		const oldEstimate = await getEstimate(id);
		await updateEstimate(id, data, lineItems);
		if (oldEstimate) {
			const changes = computeChanges(
				oldEstimate as unknown as Record<string, unknown>,
				{ ...data, notes: data.notes ?? '', status: data.status ?? 'draft', currency_code: data.currency_code ?? 'USD' },
				['estimate_number', 'client_id', 'date', 'valid_until', 'subtotal', 'tax_rate', 'total', 'notes', 'status', 'currency_code']
			);
			if (Object.keys(changes).length > 0) {
				await this._audit.logAudit({ entity_type: 'estimate', entity_id: id, action: 'update', changes, context: data.estimate_number });
			}
		}
	}

	async deleteEstimate(id: number): Promise<void> {
		const estimate = await getEstimate(id);
		await deleteEstimate(id);
		await this._audit.logAudit({ entity_type: 'estimate', entity_id: id, action: 'delete', context: estimate?.estimate_number ?? '' });
	}

	async updateEstimateStatus(id: number, status: string): Promise<void> {
		const oldEstimate = await getEstimate(id);
		await updateEstimateStatus(id, status);
		await this._audit.logAudit({
			entity_type: 'estimate',
			entity_id: id,
			action: 'status_change',
			changes: { status: { old: oldEstimate?.status ?? '', new: status } },
			context: oldEstimate?.estimate_number ?? ''
		});
	}

	async bulkDeleteEstimates(ids: number[]): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const estimates = await Promise.all(ids.map((id) => getEstimate(id)));
		await bulkDeleteEstimates(ids);
		for (let i = 0; i < ids.length; i++) {
			await this._audit.logAudit({ entity_type: 'estimate', entity_id: ids[i], action: 'delete', context: estimates[i]?.estimate_number ?? '', batch_id });
		}
	}

	async bulkUpdateEstimateStatus(ids: number[], status: string): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const estimates = await Promise.all(ids.map((id) => getEstimate(id)));
		await bulkUpdateEstimateStatus(ids, status);
		for (let i = 0; i < ids.length; i++) {
			await this._audit.logAudit({
				entity_type: 'estimate',
				entity_id: ids[i],
				action: 'status_change',
				changes: { status: { old: estimates[i]?.status ?? '', new: status } },
				context: estimates[i]?.estimate_number ?? '',
				batch_id
			});
		}
	}

	async convertEstimateToInvoice(estimateId: number): Promise<number> {
		const { invoiceId, invoiceNumber, estimateNumber } = await convertEstimateToInvoice(estimateId);
		await this._audit.logAudit({
			entity_type: 'estimate',
			entity_id: estimateId,
			action: 'convert',
			context: `${estimateNumber} -> ${invoiceNumber}`
		});
		await this._audit.logAudit({
			entity_type: 'invoice',
			entity_id: invoiceId,
			action: 'create',
			context: `${invoiceNumber} (from estimate ${estimateNumber})`
		});
		return invoiceId;
	}

	async duplicateEstimate(id: number): Promise<number> {
		const { newId, newNumber, originalNumber } = await duplicateEstimate(id);
		await this._audit.logAudit({
			entity_type: 'estimate',
			entity_id: newId,
			action: 'create',
			context: `${newNumber} (duplicated from ${originalNumber})`
		});
		return newId;
	}
}
