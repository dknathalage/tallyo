import { describe, it, expect } from 'vitest';
import { dbError, fkOrNull } from './db-error.js';

describe('dbError', () => {
	it('throws 409 for UNIQUE constraint errors', () => {
		const err = new Error('UNIQUE constraint failed: clients.email');
		expect(() => dbError(err)).toThrow();
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(409);
		}
	});

	it('includes field name in 409 message', () => {
		const err = new Error('UNIQUE constraint failed: clients.email');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { body: { message: string } }).body.message).toContain('email');
		}
	});

	it('extracts field name correctly from UNIQUE constraint message', () => {
		const err = new Error('UNIQUE constraint failed: invoices.invoice_number');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { body: { message: string } }).body.message).toContain('invoice_number');
		}
	});

	it('throws 400 for FOREIGN KEY constraint errors', () => {
		const err = new Error('FOREIGN KEY constraint failed');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(400);
			expect((e as { body: { message: string } }).body.message).toContain('Invalid reference');
		}
	});

	it('throws 400 for NOT NULL constraint errors', () => {
		const err = new Error('NOT NULL constraint failed: clients.name');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(400);
		}
	});

	it('includes field name in NOT NULL constraint message', () => {
		const err = new Error('NOT NULL constraint failed: clients.name');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { body: { message: string } }).body.message).toContain('name');
		}
	});

	it('includes "is required" in NOT NULL error message', () => {
		const err = new Error('NOT NULL constraint failed: invoices.client_id');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { body: { message: string } }).body.message).toContain('is required');
		}
	});

	it('throws 400 for "is required" error messages', () => {
		const err = new Error('Client name is required');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(400);
			expect((e as { body: { message: string } }).body.message).toBe('Client name is required');
		}
	});

	it('throws 400 for "Cannot delete" error messages', () => {
		const err = new Error('Cannot delete a client with active invoices');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(400);
			expect((e as { body: { message: string } }).body.message).toBe('Cannot delete a client with active invoices');
		}
	});

	it('throws 400 for "Cannot convert" error messages', () => {
		const err = new Error('Cannot convert an already-converted estimate');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(400);
			expect((e as { body: { message: string } }).body.message).toBe('Cannot convert an already-converted estimate');
		}
	});

	it('rethrows unknown errors as-is', () => {
		const err = new Error('some unexpected database error');
		expect(() => dbError(err)).toThrow('some unexpected database error');
	});

	it('rethrows non-Error objects', () => {
		const err = { code: 'UNKNOWN', message: 'weird error' };
		expect(() => dbError(err)).toThrow();
	});

	it('handles string errors', () => {
		expect(() => dbError('some string error')).toThrow();
	});

	it('uses "field" as fallback when UNIQUE constraint table has no dot', () => {
		const err = new Error('UNIQUE constraint failed: email');
		try {
			dbError(err);
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(409);
		}
	});
});

describe('fkOrNull', () => {
	it('returns the number for a valid positive number', () => {
		expect(fkOrNull(5)).toBe(5);
	});

	it('returns null for 0', () => {
		expect(fkOrNull(0)).toBeNull();
	});

	it('returns null for empty string', () => {
		expect(fkOrNull('')).toBeNull();
	});

	it('returns null for null', () => {
		expect(fkOrNull(null)).toBeNull();
	});

	it('returns null for undefined', () => {
		expect(fkOrNull(undefined)).toBeNull();
	});

	it('returns null for negative numbers', () => {
		expect(fkOrNull(-1)).toBeNull();
	});

	it('returns number for numeric string', () => {
		expect(fkOrNull('7')).toBe(7);
	});

	it('returns null for "0"', () => {
		expect(fkOrNull('0')).toBeNull();
	});

	it('returns null for NaN', () => {
		expect(fkOrNull(NaN)).toBeNull();
	});

	it('returns null for non-numeric string', () => {
		expect(fkOrNull('abc')).toBeNull();
	});

	it('handles positive float as a number (finite and > 0)', () => {
		expect(fkOrNull(3.5)).toBe(3.5);
	});
});
