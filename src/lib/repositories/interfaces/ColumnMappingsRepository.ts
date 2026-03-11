import type { ColumnMapping } from '$lib/types/index.js';

export interface CreateColumnMappingInput {
	name: string;
	entity_type?: string;
	mapping: Record<string, string>;
	tier_mapping?: Record<string, number>;
	metadata_mapping?: string[];
	file_type?: string;
	sheet_name?: string;
	header_row?: number;
}

export type UpdateColumnMappingInput = CreateColumnMappingInput;

export interface ColumnMappingsRepository {
	getColumnMappings(entityType?: string): ColumnMapping[];
	getColumnMapping(id: number): ColumnMapping | null;

	createColumnMapping(data: CreateColumnMappingInput): Promise<number>;
	updateColumnMapping(id: number, data: UpdateColumnMappingInput): Promise<void>;
	deleteColumnMapping(id: number): Promise<void>;
}
