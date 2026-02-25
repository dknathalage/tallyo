import Papa from 'papaparse';
import type { ValidationError } from './types.js';

export function parseCsvFile<T>(file: File): Promise<{ data: T[]; errors: Papa.ParseError[] }> {
	return new Promise((resolve) => {
		Papa.parse<T>(file, {
			header: true,
			skipEmptyLines: true,
			complete: (results) => resolve({ data: results.data, errors: results.errors })
		});
	});
}

export function validateRequiredField(value: string | undefined, field: string, row: number): ValidationError | null {
	if (!value?.trim()) return { row, field, message: `${field} is required` };
	return null;
}

export function validateNumeric(value: string | undefined, field: string, row: number): ValidationError | null {
	if (value !== undefined && value !== '' && isNaN(Number(value))) {
		return { row, field, message: `${field} must be a number` };
	}
	return null;
}

export function validateDate(value: string | undefined, field: string, row: number): ValidationError | null {
	if (value && !/^\d{4}-\d{2}-\d{2}$/.test(value)) {
		return { row, field, message: `${field} must be YYYY-MM-DD format` };
	}
	return null;
}

export function validateStatus(value: string | undefined, row: number): ValidationError | null {
	const valid = ['draft', 'sent', 'paid', 'overdue'];
	if (value && !valid.includes(value.toLowerCase())) {
		return { row, field: 'status', message: `status must be one of: ${valid.join(', ')}` };
	}
	return null;
}
