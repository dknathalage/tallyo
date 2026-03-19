import type { Estimate, EstimateLineItem, PaginationParams, PaginatedResult } from '$lib/types/index.js';
import type { CreateEstimateInput, UpdateEstimateInput, LineItemInput } from './types.js';

export interface EstimateRepository {
	getEstimates(search?: string, status?: string, pagination?: PaginationParams): Promise<PaginatedResult<Estimate>>;
	getEstimate(id: number): Promise<Estimate | null>;
	getEstimateLineItems(estimateId: number): Promise<EstimateLineItem[]>;
	getClientEstimates(clientId: number): Promise<Estimate[]>;

	createEstimate(data: CreateEstimateInput, lineItems: LineItemInput[]): Promise<number>;
	updateEstimate(id: number, data: UpdateEstimateInput, lineItems: LineItemInput[]): Promise<void>;
	deleteEstimate(id: number): Promise<void>;
	updateEstimateStatus(id: number, status: string): Promise<void>;

	bulkDeleteEstimates(ids: number[]): Promise<void>;
	bulkUpdateEstimateStatus(ids: number[], status: string): Promise<void>;

	convertEstimateToInvoice(estimateId: number): Promise<number>;
	duplicateEstimate(id: number): Promise<number>;
}
