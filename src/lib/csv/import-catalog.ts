import { query, execute, runRaw, save } from '$lib/db/connection.js';
import { parseCsvFile, validateRequiredField, validateNumeric } from './parse.js';
import type { CsvCatalogRow, ParsedImport, ValidationError } from './types.js';

export async function parseCatalogCsv(file: File): Promise<ParsedImport<CsvCatalogRow>> {
	const { data } = await parseCsvFile<CsvCatalogRow>(file);
	const totalRows = data.length;
	const errors: ValidationError[] = [];
	const validRows: CsvCatalogRow[] = [];
	let skippedDuplicates = 0;

	// Get existing UUIDs for deduplication
	const existing = query<{ uuid: string }>('SELECT uuid FROM catalog_items WHERE uuid IS NOT NULL');
	const existingUuids = new Set(existing.map((r) => r.uuid));

	for (let i = 0; i < data.length; i++) {
		const row = data[i];
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
		if (row.uuid?.trim() && existingUuids.has(row.uuid.trim())) {
			skippedDuplicates++;
			continue;
		}

		validRows.push(row);
	}

	return { validRows, errors, skippedDuplicates, totalRows };
}

export async function commitCatalogImport(rows: CsvCatalogRow[]): Promise<void> {
	try {
		runRaw('BEGIN TRANSACTION');
		for (const row of rows) {
			const uuid = row.uuid?.trim() || crypto.randomUUID();
			const rate = Number(row.rate) || 0;
			execute(
				'INSERT INTO catalog_items (uuid, name, rate, unit, category, sku) VALUES (?, ?, ?, ?, ?, ?)',
				[uuid, row.name?.trim(), rate, row.unit?.trim() || '', row.category?.trim() || '', row.sku?.trim() || '']
			);
		}
		runRaw('COMMIT');
	} catch (err) {
		runRaw('ROLLBACK');
		throw err;
	}
}
