import { getDb } from '../connection.js';
import { columnMappings } from '../drizzle-schema.js';
import { eq, asc } from 'drizzle-orm';
import type { ColumnMapping } from '../../types/index.js';

function mapRow(row: Record<string, unknown>): ColumnMapping {
	return {
		id: row['id'] as number,
		uuid: row['uuid'] as string,
		name: row['name'] as string,
		entity_type: (row['entity_type'] as string | null | undefined) ?? 'catalog',
		mapping: (row['mapping'] as string | null | undefined) ?? '{}',
		tier_mapping: (row['tier_mapping'] as string | null | undefined) ?? '{}',
		metadata_mapping: (row['metadata_mapping'] as string | null | undefined) ?? '[]',
		file_type: (row['file_type'] as string | null | undefined) ?? 'csv',
		sheet_name: (row['sheet_name'] as string | null | undefined) ?? '',
		header_row: (row['header_row'] as number | null | undefined) ?? 1,
		created_at: (row['created_at'] as string | null | undefined) ?? '',
		updated_at: (row['updated_at'] as string | null | undefined) ?? ''
	};
}

export async function getColumnMappings(entityType?: string): Promise<ColumnMapping[]> {
	const db = getDb();

	let rows;
	if (entityType) {
		rows = await db
			.select()
			.from(columnMappings)
			.where(eq(columnMappings.entity_type, entityType))
			.orderBy(asc(columnMappings.name));
	} else {
		rows = await db.select().from(columnMappings).orderBy(asc(columnMappings.name));
	}

	return rows.map((r) => mapRow(r as Record<string, unknown>));
}

export async function getColumnMapping(id: number): Promise<ColumnMapping | null> {
	const db = getDb();
	const rows = await db
		.select()
		.from(columnMappings)
		.where(eq(columnMappings.id, id));
	const first = rows[0];
	return first ? mapRow(first) : null;
}

export async function createColumnMapping(data: {
	name: string;
	entity_type?: string;
	mapping: Record<string, string>;
	tier_mapping?: Record<string, number>;
	metadata_mapping?: string[];
	file_type?: string;
	sheet_name?: string;
	header_row?: number;
}): Promise<number> {
	const db = getDb();

	const result = await db
		.insert(columnMappings)
		.values({
			uuid: crypto.randomUUID(),
			name: data.name,
			entity_type: data.entity_type ?? 'catalog',
			mapping: JSON.stringify(data.mapping),
			tier_mapping: JSON.stringify(data.tier_mapping ?? {}),
			metadata_mapping: JSON.stringify(data.metadata_mapping ?? []),
			file_type: data.file_type ?? 'csv',
			sheet_name: data.sheet_name ?? '',
			header_row: data.header_row ?? 1
		})
		.returning({ id: columnMappings.id });

	const inserted = result[0];
	if (!inserted) throw new Error('Failed to insert column mapping');
	return inserted.id;
}

export async function updateColumnMapping(
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
): Promise<void> {
	const db = getDb();

	await db
		.update(columnMappings)
		.set({
			name: data.name,
			entity_type: data.entity_type ?? 'catalog',
			mapping: JSON.stringify(data.mapping),
			tier_mapping: JSON.stringify(data.tier_mapping ?? {}),
			metadata_mapping: JSON.stringify(data.metadata_mapping ?? []),
			file_type: data.file_type ?? 'csv',
			sheet_name: data.sheet_name ?? '',
			header_row: data.header_row ?? 1,
			updated_at: new Date().toISOString()
		})
		.where(eq(columnMappings.id, id));
}

export async function deleteColumnMapping(id: number): Promise<void> {
	const db = getDb();
	await db.delete(columnMappings).where(eq(columnMappings.id, id));
}
