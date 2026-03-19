import { describe, it, expect } from 'vitest';
import { parseSnapshot } from './snapshot.js';

describe('parseSnapshot', () => {
	it('parses a full valid JSON snapshot', () => {
		const json = JSON.stringify({
			name: 'Acme Corp',
			email: 'acme@example.com',
			phone: '555-1234',
			address: '123 Main St',
			logo: 'data:image/png;base64,abc',
			metadata: { ABN: '12345' }
		});
		const result = parseSnapshot(json);
		expect(result.name).toBe('Acme Corp');
		expect(result.email).toBe('acme@example.com');
		expect(result.phone).toBe('555-1234');
		expect(result.address).toBe('123 Main St');
		expect(result.logo).toBe('data:image/png;base64,abc');
		expect(result.metadata).toEqual({ ABN: '12345' });
	});

	it('returns empty strings for missing fields', () => {
		const result = parseSnapshot('{}');
		expect(result.name).toBe('');
		expect(result.email).toBe('');
		expect(result.phone).toBe('');
		expect(result.address).toBe('');
		expect(result.metadata).toEqual({});
	});

	it('returns undefined logo when not present', () => {
		const result = parseSnapshot('{}');
		expect(result.logo).toBeUndefined();
	});

	it('handles empty string input', () => {
		const result = parseSnapshot('');
		expect(result.name).toBe('');
		expect(result.email).toBe('');
		expect(result.metadata).toEqual({});
	});

	it('handles invalid JSON by returning empty snapshot', () => {
		const result = parseSnapshot('not valid json {{{');
		expect(result.name).toBe('');
		expect(result.email).toBe('');
		expect(result.phone).toBe('');
		expect(result.address).toBe('');
		expect(result.metadata).toEqual({});
	});

	it('handles null-like JSON (JSON null string) gracefully', () => {
		// JSON.parse('null') returns null, which is not an object with properties
		// The code does `parsed.name || ''` which would throw on null
		// Actually JSON.parse('null') = null, then null.name throws -> catch returns default
		const result = parseSnapshot('null');
		expect(result.name).toBe('');
	});

	it('handles partial data - only name provided', () => {
		const result = parseSnapshot(JSON.stringify({ name: 'Bob' }));
		expect(result.name).toBe('Bob');
		expect(result.email).toBe('');
		expect(result.phone).toBe('');
		expect(result.address).toBe('');
	});

	it('handles metadata with multiple entries', () => {
		const json = JSON.stringify({ name: 'Test', metadata: { ABN: '123', VAT: 'GB456' } });
		const result = parseSnapshot(json);
		expect(result.metadata).toEqual({ ABN: '123', VAT: 'GB456' });
	});

	it('preserves logo field when present', () => {
		const json = JSON.stringify({ logo: 'http://example.com/logo.png' });
		const result = parseSnapshot(json);
		expect(result.logo).toBe('http://example.com/logo.png');
	});

	it('handles empty metadata field as empty object', () => {
		const json = JSON.stringify({ name: 'A', metadata: null });
		const result = parseSnapshot(json);
		// null || {} = {}
		expect(result.metadata).toEqual({});
	});

	it('handles multiline address', () => {
		const json = JSON.stringify({ address: '123 Main St\nCity, ST 12345' });
		const result = parseSnapshot(json);
		expect(result.address).toBe('123 Main St\nCity, ST 12345');
	});
});
