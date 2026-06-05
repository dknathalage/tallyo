import { createCollectionStore } from './collection.svelte';
import type { ColumnMapping, ColumnMappingInput } from '$lib/api/types';

export const columnMappings = createCollectionStore<ColumnMapping, ColumnMappingInput>(
	'column-mappings',
	'column_mapping'
);
