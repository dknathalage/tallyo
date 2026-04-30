import { parseCsvFile, validateRequiredField } from './parse.js';
import type { CsvClientRow, ParsedImport, ValidationError } from './types.js';
import type { ClientRepository } from '$lib/repositories/interfaces/index.js';

export async function parseClientsCsv(file: File): Promise<ParsedImport<CsvClientRow>> {
	const { data } = await parseCsvFile<CsvClientRow>(file);
	const totalRows = data.length;
	const errors: ValidationError[] = [];
	const validRows: CsvClientRow[] = [];
	let skippedDuplicates = 0;

	// Get existing UUIDs for deduplication via API
	const existingRes = await fetch('/api/clients');
	const existingClients = await existingRes.json() as Array<{ uuid: string }>;
	const existingUuids = new Set(existingClients.map((r) => r.uuid).filter(Boolean));

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

/**
 * Commits the parsed client import through the repository layer.
 * The repository layer handles audit logging for each record.
 */
export async function commitClientImport(
	rows: CsvClientRow[],
	repos: { clients: ClientRepository }
): Promise<void> {
	for (const row of rows) {
		await repos.clients.createClient({
			uuid: row.uuid?.trim() || undefined,
			name: row.name?.trim() || '',
			email: row.email?.trim() || '',
			phone: row.phone?.trim() || '',
			address: row.address?.trim() || ''
		});
	}
}
