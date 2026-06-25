import { createCollectionStore } from './collection.svelte';
import type { CatalogueItem, CatalogueItemInput } from '$lib/api/types';

// The per-tenant catalogue: reusable priced line templates with per-item
// copy-on-write versioning. List/query return current rows; the CRUD helper
// drives the DataTable and the line-item pickers. Refetches on the
// `catalogue_item` SSE event.
export const catalogue = createCollectionStore<CatalogueItem, CatalogueItemInput>(
	'catalogue',
	'catalogue_item'
);
