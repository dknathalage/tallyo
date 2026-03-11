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
	getColumnMappings(entityType?: string): ColumnMapping[] {
		return getColumnMappings(entityType);
	}

	getColumnMapping(id: number): ColumnMapping | null {
		return getColumnMapping(id);
	}

	createColumnMapping(data: CreateColumnMappingInput): Promise<number> {
		return createColumnMapping(data);
	}

	updateColumnMapping(id: number, data: UpdateColumnMappingInput): Promise<void> {
		return updateColumnMapping(id, data);
	}

	deleteColumnMapping(id: number): Promise<void> {
		return deleteColumnMapping(id);
	}
}
