import type { Estimate, EstimateLineItem } from '$lib/types/index.js';
import type { CreateEstimateInput, UpdateEstimateInput, LineItemInput } from './types.js';

export interface EstimateRepository {
	getEstimates(search?: string, status?: string): Estimate[];
	getEstimate(id: number): Estimate | null;
	getEstimateLineItems(estimateId: number): EstimateLineItem[];
	getClientEstimates(clientId: number): Estimate[];

	createEstimate(data: CreateEstimateInput, lineItems: LineItemInput[]): Promise<number>;
	updateEstimate(id: number, data: UpdateEstimateInput, lineItems: LineItemInput[]): Promise<void>;
	deleteEstimate(id: number): Promise<void>;
	updateEstimateStatus(id: number, status: string): Promise<void>;

	bulkDeleteEstimates(ids: number[]): Promise<void>;
	bulkUpdateEstimateStatus(ids: number[], status: string): Promise<void>;

	convertEstimateToInvoice(estimateId: number): Promise<number>;
}
