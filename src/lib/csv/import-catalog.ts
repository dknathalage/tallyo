import { getDb } from '$lib/db/connection.js';
import { catalogItems } from '$lib/db/drizzle-schema.js';
import { parseCsvFile, validateRequiredField, validateNumeric } from './parse.js';
import type { CsvCatalogRow, ParsedImport, ValidationError } from './types.js';

export async function parseCatalogCsv(file: File): Promise<ParsedImport<CsvCatalogRow>> {
	const { data } = await parseCsvFile<CsvCatalogRow>(file);
	const totalRows = data.length;
	const errors: ValidationError[] = [];
	const validRows: CsvCatalogRow[] = [];
	let skippedDuplicates = 0;

	// Get existing UUIDs for deduplication
	const db = getDb();
	const existing = await db.select({ uuid: catalogItems.uuid }).from(catalogItems);
	const existingUuids = new Set(existing.map((r) => r.uuid).filter(Boolean));

	for (let i = 0; i < data.length; i++) {
		const row = data[i];
		if (!row) continue;
		const rowNum = i + 1;
		let hasError = false;

		// Validate required fields
		const nameErr = validateRequiredField(row.name, 'name', rowNum);
		if (nameErr) {
			errors.push(nameErr);
			hasError = true;
		}

		// Validate numeric fields
		const rateErr = validateNumeric(row.rate, 'rate', rowNum);
		if (rateErr) {
			errors.push(rateErr);
			hasError = true;
		}

		if (hasError) continue;

		// Skip duplicates by UUID
		if (row.uuid.trim() && existingUuids.has(row.uuid.trim())) {
			skippedDuplicates++;
			continue;
		}

		validRows.push(row);
	}

	return { validRows, errors, skippedDuplicates, totalRows };
}

export async function commitCatalogImport(rows: CsvCatalogRow[]): Promise<void> {
	const db = getDb();

	for (const row of rows) {
		const uuid = row.uuid.trim() || crypto.randomUUID();
		const rate = Number(row.rate) || 0;
		await db.insert(catalogItems).values({
			uuid,
			name: row.name.trim(),
			rate,
			unit: row.unit.trim(),
			category: row.category.trim(),
			sku: row.sku.trim()
		});
	}
}
