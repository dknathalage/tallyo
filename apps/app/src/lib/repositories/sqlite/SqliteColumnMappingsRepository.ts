import {
	getColumnMappings,
	getColumnMapping,
	createColumnMapping,
	updateColumnMapping,
	deleteColumnMapping
} from '$lib/db/queries/column-mappings.js';
import type { ColumnMappingsRepository, CreateColumnMappingInput, UpdateColumnMappingInput } from '../interfaces/ColumnMappingsRepository.js';
import type { ColumnMapping } from '$lib/types/index.js';

export class SqliteColumnMappingsRepository implements ColumnMappingsRepository {
	async getColumnMappings(entityType?: string): Promise<ColumnMapping[]> {
		return await getColumnMappings(entityType);
	}

	async getColumnMapping(id: number): Promise<ColumnMapping | null> {
		return await getColumnMapping(id);
	}

	async createColumnMapping(data: CreateColumnMappingInput): Promise<number> {
		return await createColumnMapping(data);
	}

	async updateColumnMapping(id: number, data: UpdateColumnMappingInput): Promise<void> {
		return await updateColumnMapping(id, data);
	}

	async deleteColumnMapping(id: number): Promise<void> {
		return await deleteColumnMapping(id);
	}
}
