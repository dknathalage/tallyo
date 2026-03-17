import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('papaparse', () => ({
	default: {
		parse: vi.fn()
	}
}));

import Papa from 'papaparse';
import {
	parseCsvFile,
	validateRequiredField,
	validateNumeric,
	validateDate,
	validateStatus
} from './parse.js';

const mockPapaParse = vi.mocked(Papa.parse) as any;

beforeEach(() => {
	vi.clearAllMocks();
});

describe('parseCsvFile', () => {
	it('resolves with parsed data on success', async () => {
		const fakeData = [{ name: 'Alice' }, { name: 'Bob' }];
		mockPapaParse.mockImplementation((_file: unknown, opts: any) => {
			opts.complete({ data: fakeData, errors: [] });
		});
		const file = new File(['name\nAlice\nBob'], 'test.csv', { type: 'text/csv' });
		const result = await parseCsvFile(file);
		expect(result.data).toEqual(fakeData);
		expect(result.errors).toEqual([]);
	});

	it('resolves with errors when parse errors occur', async () => {
		const fakeErrors = [{ type: 'Delimiter', code: 'UndetectableDelimiter', message: 'fail', row: 0 }];
		mockPapaParse.mockImplementation((_file: unknown, opts: any) => {
			opts.complete({ data: [], errors: fakeErrors });
		});
		const file = new File([''], 'empty.csv', { type: 'text/csv' });
		const result = await parseCsvFile(file);
		expect(result.data).toEqual([]);
		expect(result.errors).toEqual(fakeErrors);
	});

	it('calls Papa.parse with header:true and skipEmptyLines:true', async () => {
		mockPapaParse.mockImplementation((_file: unknown, opts: any) => {
			opts.complete({ data: [], errors: [] });
		});
		const file = new File([''], 'test.csv');
		await parseCsvFile(file);
		expect(mockPapaParse).toHaveBeenCalledWith(
			file,
			expect.objectContaining({ header: true, skipEmptyLines: true })
		);
	});
});

describe('validateRequiredField', () => {
	it('returns null when value is present', () => {
		expect(validateRequiredField('Alice', 'name', 1)).toBeNull();
	});

	it('returns error when value is empty string', () => {
		const err = validateRequiredField('', 'name', 2);
		expect(err).not.toBeNull();
		expect(err!.row).toBe(2);
		expect(err!.field).toBe('name');
		expect(err!.message).toContain('name is required');
	});

	it('returns error when value is whitespace only', () => {
		const err = validateRequiredField('   ', 'email', 3);
		expect(err).not.toBeNull();
		expect(err!.field).toBe('email');
	});

	it('returns error when value is undefined', () => {
		const err = validateRequiredField(undefined, 'name', 1);
		expect(err).not.toBeNull();
		expect(err!.message).toBe('name is required');
	});

	it('returns null when value has leading/trailing whitespace but content', () => {
		expect(validateRequiredField('  Alice  ', 'name', 1)).toBeNull();
	});
});

describe('validateNumeric', () => {
	it('returns null for a valid numeric string', () => {
		expect(validateNumeric('42', 'rate', 1)).toBeNull();
	});

	it('returns null for a decimal number', () => {
		expect(validateNumeric('3.14', 'rate', 1)).toBeNull();
	});

	it('returns null for an empty string (optional field)', () => {
		expect(validateNumeric('', 'rate', 1)).toBeNull();
	});

	it('returns null for undefined (optional field)', () => {
		expect(validateNumeric(undefined, 'rate', 1)).toBeNull();
	});

	it('returns error for a non-numeric string', () => {
		const err = validateNumeric('abc', 'rate', 2);
		expect(err).not.toBeNull();
		expect(err!.row).toBe(2);
		expect(err!.field).toBe('rate');
		expect(err!.message).toContain('must be a number');
	});

	it('returns error for mixed alphanumeric', () => {
		const err = validateNumeric('12abc', 'quantity', 5);
		expect(err).not.toBeNull();
	});

	it('returns null for zero', () => {
		expect(validateNumeric('0', 'rate', 1)).toBeNull();
	});

	it('returns null for negative number', () => {
		expect(validateNumeric('-5', 'rate', 1)).toBeNull();
	});
});

describe('validateDate', () => {
	it('returns null for valid YYYY-MM-DD date', () => {
		expect(validateDate('2024-01-15', 'date', 1)).toBeNull();
	});

	it('returns null for undefined (optional field)', () => {
		expect(validateDate(undefined, 'date', 1)).toBeNull();
	});

	it('returns null for empty string (optional)', () => {
		expect(validateDate('', 'date', 1)).toBeNull();
	});

	it('returns error for DD/MM/YYYY format', () => {
		const err = validateDate('15/01/2024', 'date', 3);
		expect(err).not.toBeNull();
		expect(err!.field).toBe('date');
		expect(err!.message).toContain('YYYY-MM-DD');
	});

	it('returns error for MM-DD-YYYY format', () => {
		const err = validateDate('01-15-2024', 'due_date', 4);
		expect(err).not.toBeNull();
	});

	it('returns error for plain text', () => {
		const err = validateDate('January 15', 'date', 1);
		expect(err).not.toBeNull();
	});

	it('returns null for a value like 2024-12-31', () => {
		expect(validateDate('2024-12-31', 'date', 1)).toBeNull();
	});
});

describe('validateStatus', () => {
	it('returns null for draft', () => {
		expect(validateStatus('draft', 1)).toBeNull();
	});

	it('returns null for sent', () => {
		expect(validateStatus('sent', 1)).toBeNull();
	});

	it('returns null for paid', () => {
		expect(validateStatus('paid', 1)).toBeNull();
	});

	it('returns null for overdue', () => {
		expect(validateStatus('overdue', 1)).toBeNull();
	});

	it('is case-insensitive', () => {
		expect(validateStatus('DRAFT', 1)).toBeNull();
		expect(validateStatus('Paid', 1)).toBeNull();
	});

	it('returns null for undefined (optional)', () => {
		expect(validateStatus(undefined, 1)).toBeNull();
	});

	it('returns error for invalid status', () => {
		const err = validateStatus('cancelled', 2);
		expect(err).not.toBeNull();
		expect(err!.field).toBe('status');
		expect(err!.message).toContain('draft, sent, paid, overdue');
	});

	it('returns error for random string', () => {
		const err = validateStatus('unknown', 5);
		expect(err).not.toBeNull();
		expect(err!.row).toBe(5);
	});
});
