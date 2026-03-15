import { execute, query, save } from '../connection.js';
import type { ColumnMapping } from '../../types/index.js';

export function getColumnMappings(entityType?: string): ColumnMapping[] {
	if (entityType) {
		return query<ColumnMapping>(
			`SELECT * FROM column_mappings WHERE entity_type = ? ORDER BY name`,
			[entityType]
		);
	}
	return query<ColumnMapping>(`SELECT * FROM column_mappings ORDER BY name`);
}

export function getColumnMapping(id: number): ColumnMapping | null {
	const results = query<ColumnMapping>(`SELECT * FROM column_mappings WHERE id = ?`, [id]);
	return results.length > 0 ? results[0] : null;
}

export function createColumnMapping(data: {
	name: string;
	entity_type?: string;
	mapping: Record<string, string>;
	tier_mapping?: Record<string, number>;
	metadata_mapping?: string[];
	file_type?: string;
	sheet_name?: string;
	header_row?: number;
}): number {
	execute(
		`INSERT INTO column_mappings (uuid, name, entity_type, mapping, tier_mapping, metadata_mapping, file_type, sheet_name, header_row) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[
			crypto.randomUUID(),
			data.name,
			data.entity_type ?? 'catalog',
			JSON.stringify(data.mapping),
			JSON.stringify(data.tier_mapping ?? {}),
			JSON.stringify(data.metadata_mapping ?? []),
			data.file_type ?? 'csv',
			data.sheet_name ?? '',
			data.header_row ?? 1
		]
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	return result[0].id;
}

export function updateColumnMapping(
	id: number,
	data: {
		name: string;
		entity_type?: string;
		mapping: Record<string, string>;
		tier_mapping?: Record<string, number>;
		metadata_mapping?: string[];
		file_type?: string;
		sheet_name?: string;
		header_row?: number;
	}
): void {
	execute(
		`UPDATE column_mappings SET name = ?, entity_type = ?, mapping = ?, tier_mapping = ?, metadata_mapping = ?, file_type = ?, sheet_name = ?, header_row = ?, updated_at = datetime('now') WHERE id = ?`,
		[
			data.name,
			data.entity_type ?? 'catalog',
			JSON.stringify(data.mapping),
			JSON.stringify(data.tier_mapping ?? {}),
			JSON.stringify(data.metadata_mapping ?? []),
			data.file_type ?? 'csv',
			data.sheet_name ?? '',
			data.header_row ?? 1,
			id
		]
	);
}

export function deleteColumnMapping(id: number): void {
	execute(`DELETE FROM column_mappings WHERE id = ?`, [id]);
}
