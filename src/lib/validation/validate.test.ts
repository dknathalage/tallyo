import { describe, it, expect } from 'vitest';
import { z } from 'zod';
import { validate } from './validate.js';

const SimpleSchema = z.object({
	name: z.string().min(1, 'Name is required'),
	age: z.number().int().positive('Age must be positive')
});

describe('validate', () => {
	it('returns parsed data when valid', () => {
		const result = validate(SimpleSchema, { name: 'Alice', age: 30 });
		expect(result).toEqual({ name: 'Alice', age: 30 });
	});

	it('throws 400 error when data is invalid', () => {
		expect(() => validate(SimpleSchema, {})).toThrow();
	});

	it('throws error with status 400', () => {
		try {
			validate(SimpleSchema, { name: '', age: 30 });
			expect.fail('should have thrown');
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(400);
		}
	});

	it('includes field path in error message', () => {
		try {
			validate(SimpleSchema, { name: '', age: 30 });
			expect.fail('should have thrown');
		} catch (e: unknown) {
			expect((e as { body: { message: string } }).body.message).toContain('name');
		}
	});

	it('includes Zod error message in error body', () => {
		try {
			validate(SimpleSchema, { name: '', age: 30 });
			expect.fail('should have thrown');
		} catch (e: unknown) {
			expect((e as { body: { message: string } }).body.message).toContain('Name is required');
		}
	});

	it('includes "Validation failed:" prefix in the error message', () => {
		try {
			validate(SimpleSchema, { name: 'Alice', age: -1 });
			expect.fail('should have thrown');
		} catch (e: unknown) {
			expect((e as { body: { message: string } }).body.message).toContain('Validation failed:');
		}
	});

	it('combines multiple validation errors into one message', () => {
		try {
			validate(SimpleSchema, { name: '', age: -1 });
			expect.fail('should have thrown');
		} catch (e: unknown) {
			const msg = (e as { body: { message: string } }).body.message;
			expect(msg).toContain('name');
			expect(msg).toContain('age');
		}
	});

	it('works with nested schema paths', () => {
		const NestedSchema = z.object({
			user: z.object({
				email: z.string().email('Invalid email')
			})
		});
		try {
			validate(NestedSchema, { user: { email: 'not-valid' } });
			expect.fail('should have thrown');
		} catch (e: unknown) {
			const msg = (e as { body: { message: string } }).body.message;
			expect(msg).toContain('user.email');
		}
	});

	it('handles non-object schemas', () => {
		const StringSchema = z.string().min(3, 'Too short');
		try {
			validate(StringSchema, 'ab');
			expect.fail('should have thrown');
		} catch (e: unknown) {
			expect((e as { status: number }).status).toBe(400);
			expect((e as { body: { message: string } }).body.message).toContain('Too short');
		}
	});

	it('returns correct typed data', () => {
		const data = validate(SimpleSchema, { name: 'Bob', age: 25 });
		expect(data.name).toBe('Bob');
		expect(data.age).toBe(25);
	});
});
