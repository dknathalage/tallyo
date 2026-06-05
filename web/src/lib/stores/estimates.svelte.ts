import { createCollectionStore } from './collection.svelte';
import type { Estimate, EstimateInput } from '$lib/api/types';

// The create/update payload is the estimate input fields plus its line items.
// The generic CRUD sends this object as-is; the server is authoritative on
// totals and snapshots.
export type EstimateCreatePayload = EstimateInput;

export const estimates = createCollectionStore<Estimate, EstimateCreatePayload>(
	'estimates',
	'estimate'
);
