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
import type { EstimateRepository } from '../interfaces/EstimateRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { StorageTransaction } from '../interfaces/StorageTransaction.js';
import type { CreateEstimateInput, UpdateEstimateInput, LineItemInput } from '../interfaces/types.js';
import type { Estimate, EstimateLineItem } from '$lib/types/index.js';

export class SqliteEstimateRepository implements EstimateRepository {
	constructor(
		private readonly _audit?: AuditRepository,
		private readonly _tx?: StorageTransaction
	) {}

	getEstimates(search?: string, status?: string): Estimate[] {
		return getEstimates(search, status);
	}

	getEstimate(id: number): Estimate | null {
		return getEstimate(id);
	}

	getEstimateLineItems(estimateId: number): EstimateLineItem[] {
		return getEstimateLineItems(estimateId);
	}

	getClientEstimates(clientId: number): Estimate[] {
		return getClientEstimates(clientId);
	}

	createEstimate(data: CreateEstimateInput, lineItems: LineItemInput[]): Promise<number> {
		return createEstimate(data, lineItems);
	}

	updateEstimate(id: number, data: UpdateEstimateInput, lineItems: LineItemInput[]): Promise<void> {
		return updateEstimate(id, data, lineItems);
	}

	deleteEstimate(id: number): Promise<void> {
		return deleteEstimate(id);
	}

	updateEstimateStatus(id: number, status: string): Promise<void> {
		return updateEstimateStatus(id, status);
	}

	bulkDeleteEstimates(ids: number[]): Promise<void> {
		return bulkDeleteEstimates(ids);
	}

	bulkUpdateEstimateStatus(ids: number[], status: string): Promise<void> {
		return bulkUpdateEstimateStatus(ids, status);
	}

	convertEstimateToInvoice(estimateId: number): Promise<number> {
		return convertEstimateToInvoice(estimateId);
	}

	duplicateEstimate(id: number): Promise<number> {
		return duplicateEstimate(id);
	}
}
