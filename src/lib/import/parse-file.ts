import Papa from 'papaparse';
import * as XLSX from 'xlsx';

export interface ParsedSheet {
	headers: string[];
	rows: Record<string, string>[];
	sheetName: string;
}

export interface ParsedFile {
	sheets: ParsedSheet[];
	fileName: string;
	fileType: 'csv' | 'xlsx';
}

export async function parseFile(file: File): Promise<ParsedFile> {
	const ext = file.name.split('.').pop()?.toLowerCase();

	if (ext === 'csv') {
		return parseCsv(file);
	}
	if (ext === 'xlsx' || ext === 'xls') {
		return parseXlsx(file);
	}

	throw new Error(`Unsupported file type: .${ext}`);
}

function parseCsv(file: File): Promise<ParsedFile> {
	return new Promise((resolve, reject) => {
		Papa.parse(file, {
			header: true,
			skipEmptyLines: true,
			complete: (results) => {
				const headers = results.meta.fields ?? [];
				const rows = results.data as Record<string, string>[];
				resolve({
					sheets: [{ headers, rows, sheetName: 'Sheet1' }],
					fileName: file.name,
					fileType: 'csv'
				});
			},
			error: (err: Error) => reject(err)
		});
	});
}

async function parseXlsx(file: File): Promise<ParsedFile> {
	const buffer = await file.arrayBuffer();
	const workbook = XLSX.read(buffer, { type: 'array' });

	const sheets: ParsedSheet[] = [];
	for (const sheetName of workbook.SheetNames) {
		const worksheet = workbook.Sheets[sheetName];
		const jsonData = XLSX.utils.sheet_to_json<Record<string, unknown>>(worksheet, {
			header: 1,
			defval: '',
			raw: false
		});

		if (jsonData.length === 0) continue;

		const rawRows = jsonData as unknown as string[][];
		const headers = rawRows[0].map((h) => String(h ?? '').trim());
		const rows: Record<string, string>[] = [];

		for (let i = 1; i < rawRows.length; i++) {
			const row: Record<string, string> = {};
			let hasValue = false;
			for (let j = 0; j < headers.length; j++) {
				const val = String(rawRows[i]?.[j] ?? '').trim();
				row[headers[j]] = val;
				if (val) hasValue = true;
			}
			if (hasValue) rows.push(row);
		}

		sheets.push({ headers, rows, sheetName });
	}

	return {
		sheets,
		fileName: file.name,
		fileType: 'xlsx'
	};
}

export function getSheetWithHeaderRow(sheet: ParsedSheet, headerRow: number): ParsedSheet {
	if (headerRow <= 1) return sheet;

	const allRows = [
		Object.fromEntries(sheet.headers.map((h, i) => [String(i), h])),
		...sheet.rows.map((row) =>
			Object.fromEntries(sheet.headers.map((h, i) => [String(i), row[h] ?? '']))
		)
	];

	const headerIdx = headerRow - 1;
	if (headerIdx >= allRows.length) return sheet;

	const headerValues = Object.values(allRows[headerIdx]);
	const newHeaders = headerValues.map((h) => String(h ?? '').trim());

	const newRows: Record<string, string>[] = [];
	for (let i = headerIdx + 1; i < allRows.length; i++) {
		const rawValues = Object.values(allRows[i]);
		const row: Record<string, string> = {};
		let hasValue = false;
		for (let j = 0; j < newHeaders.length; j++) {
			const val = String(rawValues[j] ?? '').trim();
			row[newHeaders[j]] = val;
			if (val) hasValue = true;
		}
		if (hasValue) newRows.push(row);
	}

	return {
		headers: newHeaders,
		rows: newRows,
		sheetName: sheet.sheetName
	};
}
