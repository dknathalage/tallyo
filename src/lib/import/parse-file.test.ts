import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('papaparse', () => ({
	default: { parse: vi.fn() }
}));

vi.mock('xlsx', () => ({
	read: vi.fn(),
	utils: { sheet_to_json: vi.fn() }
}));

import Papa from 'papaparse';
import * as XLSX from 'xlsx';
import { parseFile, getSheetWithHeaderRow } from './parse-file.js';

const mockPapaParse = vi.mocked(Papa.parse) as any;
const mockXlsxRead = vi.mocked(XLSX.read);
const mockSheetToJson = vi.mocked(XLSX.utils.sheet_to_json);

beforeEach(() => {
	vi.clearAllMocks();
});

function makeFile(name: string, content = ''): File {
	return new File([content], name, { type: 'text/csv' });
}

describe('parseFile', () => {
	it('throws for unsupported file types', async () => {
		await expect(parseFile(makeFile('data.txt'))).rejects.toThrow('Unsupported file type: .txt');
	});

	it('throws for .pdf extension', async () => {
		await expect(parseFile(makeFile('data.pdf'))).rejects.toThrow('Unsupported file type: .pdf');
	});

	describe('CSV files', () => {
		it('parses a CSV file and returns single sheet named Sheet1', async () => {
			mockPapaParse.mockImplementation((_file: unknown, opts: any) => {
				opts.complete({ data: [{ name: 'Item A', rate: '10' }], meta: { fields: ['name', 'rate'] }, errors: [] });
			});
			const result = await parseFile(makeFile('catalog.csv'));
			expect(result.fileType).toBe('csv');
			expect(result.fileName).toBe('catalog.csv');
			expect(result.sheets).toHaveLength(1);
			expect(result.sheets[0]?.sheetName).toBe('Sheet1');
		});

		it('extracts headers from CSV meta fields', async () => {
			mockPapaParse.mockImplementation((_file: unknown, opts: any) => {
				opts.complete({ data: [], meta: { fields: ['col1', 'col2', 'col3'] }, errors: [] });
			});
			const result = await parseFile(makeFile('test.csv'));
			expect(result.sheets[0]?.headers).toEqual(['col1', 'col2', 'col3']);
		});

		it('returns rows from CSV data', async () => {
			const rows = [{ name: 'A', rate: '10' }, { name: 'B', rate: '20' }];
			mockPapaParse.mockImplementation((_file: unknown, opts: any) => {
				opts.complete({ data: rows, meta: { fields: ['name', 'rate'] }, errors: [] });
			});
			const result = await parseFile(makeFile('test.csv'));
			expect(result.sheets[0]?.rows).toEqual(rows);
		});

		it('rejects when Papa.parse calls error callback', async () => {
			mockPapaParse.mockImplementation((_file: unknown, opts: any) => {
				opts.error(new Error('Parse failed'));
			});
			await expect(parseFile(makeFile('bad.csv'))).rejects.toThrow('Parse failed');
		});

		it('defaults headers to empty array when meta.fields is null', async () => {
			mockPapaParse.mockImplementation((_file: unknown, opts: any) => {
				opts.complete({ data: [], meta: { fields: null }, errors: [] });
			});
			const result = await parseFile(makeFile('test.csv'));
			expect(result.sheets[0]?.headers).toEqual([]);
		});
	});

	describe('XLSX files', () => {
		it('parses an xlsx file and returns xlsx fileType', async () => {
			mockXlsxRead.mockReturnValue({ SheetNames: ['Sheet1'], Sheets: { Sheet1: {} } });
			mockSheetToJson.mockReturnValue([['Name', 'Rate'], ['Item A', '100'], ['Item B', '200']] as any);
			const file = Object.assign(makeFile('catalog.xlsx'), {
				arrayBuffer: vi.fn().mockResolvedValue(new ArrayBuffer(0))
			});
			const result = await parseFile(file);
			expect(result.fileType).toBe('xlsx');
			expect(result.fileName).toBe('catalog.xlsx');
		});

		it('extracts sheet names from workbook', async () => {
			mockXlsxRead.mockReturnValue({ SheetNames: ['Products', 'Services'], Sheets: { Products: {}, Services: {} } });
			mockSheetToJson
				.mockReturnValueOnce([['Name'], ['Item A']] as any)
				.mockReturnValueOnce([['Name'], ['Service B']] as any);
			const file = Object.assign(makeFile('data.xlsx'), {
				arrayBuffer: vi.fn().mockResolvedValue(new ArrayBuffer(0))
			});
			const result = await parseFile(file);
			expect(result.sheets).toHaveLength(2);
			expect(result.sheets[0]?.sheetName).toBe('Products');
		});

		it('skips empty sheets', async () => {
			mockXlsxRead.mockReturnValue({ SheetNames: ['Empty', 'Full'], Sheets: { Empty: {}, Full: {} } });
			mockSheetToJson
				.mockReturnValueOnce([] as any)
				.mockReturnValueOnce([['Name'], ['Item A']] as any);
			const file = Object.assign(makeFile('data.xlsx'), {
				arrayBuffer: vi.fn().mockResolvedValue(new ArrayBuffer(0))
			});
			const result = await parseFile(file);
			expect(result.sheets).toHaveLength(1);
			expect(result.sheets[0]?.sheetName).toBe('Full');
		});

		it('also accepts .xls files', async () => {
			mockXlsxRead.mockReturnValue({ SheetNames: ['Sheet1'], Sheets: { Sheet1: {} } });
			mockSheetToJson.mockReturnValue([['Name'], ['Item']] as any);
			const file = Object.assign(makeFile('data.xls'), {
				arrayBuffer: vi.fn().mockResolvedValue(new ArrayBuffer(0))
			});
			const result = await parseFile(file);
			expect(result.fileType).toBe('xlsx');
		});

		it('skips empty rows in xlsx data', async () => {
			mockXlsxRead.mockReturnValue({ SheetNames: ['Sheet1'], Sheets: { Sheet1: {} } });
			mockSheetToJson.mockReturnValue([
				['Name', 'Rate'],
				['', ''],
				['Item A', '50']
			] as any);
			const file = Object.assign(makeFile('data.xlsx'), {
				arrayBuffer: vi.fn().mockResolvedValue(new ArrayBuffer(0))
			});
			const result = await parseFile(file);
			expect(result.sheets[0]?.rows).toHaveLength(1);
			expect(result.sheets[0]?.rows[0]?.['Name']).toBe('Item A');
		});
	});
});

describe('getSheetWithHeaderRow', () => {
	const baseSheet = {
		sheetName: 'Sheet1',
		headers: ['col0', 'col1', 'col2'],
		rows: [
			{ col0: 'Skip Row 1', col1: 'Skip Row 1', col2: 'Skip Row 1' },
			{ col0: 'Real Header A', col1: 'Real Header B', col2: 'Real Header C' },
			{ col0: 'Data1A', col1: 'Data1B', col2: 'Data1C' },
			{ col0: 'Data2A', col1: 'Data2B', col2: 'Data2C' }
		]
	};

	it('returns the original sheet when headerRow is 1', () => {
		const result = getSheetWithHeaderRow(baseSheet, 1);
		expect(result).toBe(baseSheet);
	});

	it('returns the original sheet when headerRow is <= 1', () => {
		const result = getSheetWithHeaderRow(baseSheet, 0);
		expect(result).toBe(baseSheet);
	});

	it('uses appropriate row as headers when headerRow is 2', () => {
		const result = getSheetWithHeaderRow(baseSheet, 2);
		expect(result.headers).toContain('Skip Row 1');
	});

	it('returns original sheet when headerRow exceeds total rows', () => {
		const result = getSheetWithHeaderRow(baseSheet, 100);
		expect(result).toBe(baseSheet);
	});

	it('preserves sheetName', () => {
		const result = getSheetWithHeaderRow(baseSheet, 2);
		expect(result.sheetName).toBe('Sheet1');
	});

	it('uses row 3 as header and includes data rows after it', () => {
		const result = getSheetWithHeaderRow(baseSheet, 3);
		expect(result.headers).toContain('Real Header A');
		expect(result.rows).toHaveLength(2);
	});

	it('handles a sheet with a single row (only headers, no data)', () => {
		const singleRowSheet = {
			sheetName: 'Sheet1',
			headers: ['h0'],
			rows: [{ h0: 'actual header' }]
		};
		const result = getSheetWithHeaderRow(singleRowSheet, 2);
		expect(result.rows).toHaveLength(0);
	});
});
