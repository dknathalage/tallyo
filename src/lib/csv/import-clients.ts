import { query, execute, runRaw, save } from '$lib/db/connection.svelte.js';
import { parseCsvFile, validateRequiredField } from './parse.js';
import type { CsvClientRow, ParsedImport, ValidationError } from './types.js';

export async function parseClientsCsv(file: File): Promise<ParsedImport<CsvClientRow>> {
	const { data } = await parseCsvFile<CsvClientRow>(file);
	const totalRows = data.length;
	const errors: ValidationError[] = [];
	const validRows: CsvClientRow[] = [];
	let skippedDuplicates = 0;

	// Get existing UUIDs for deduplication
	const existing = query<{ uuid: string }>('SELECT uuid FROM clients WHERE uuid IS NOT NULL');
	const existingUuids = new Set(existing.map((r) => r.uuid));

	for (let i = 0; i < data.length; i++) {
		const row = data[i];
		const rowNum = i + 1;

		// Validate required fields
		const nameErr = validateRequiredField(row.name, 'name', rowNum);
		if (nameErr) {
			errors.push(nameErr);
			continue;
		}

		// Skip duplicates by UUID
		if (row.uuid?.trim() && existingUuids.has(row.uuid.trim())) {
			skippedDuplicates++;
			continue;
		}

		validRows.push(row);
	}

	return { validRows, errors, skippedDuplicates, totalRows };
}

export async function commitClientImport(rows: CsvClientRow[]): Promise<void> {
	try {
		runRaw('BEGIN TRANSACTION');
		for (const row of rows) {
			const uuid = row.uuid?.trim() || crypto.randomUUID();
			execute(
				'INSERT INTO clients (uuid, name, email, phone, address) VALUES (?, ?, ?, ?, ?)',
				[uuid, row.name?.trim(), row.email?.trim() || '', row.phone?.trim() || '', row.address?.trim() || '']
			);
		}
		runRaw('COMMIT');
		await save();
	} catch (err) {
		runRaw('ROLLBACK');
		throw err;
	}
}
