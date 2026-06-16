import { describe, it, expect } from 'vitest';
import { chooseRenderer, tableColumns, formatCell } from './resultRender';

// ---------------------------------------------------------------------------
// formatCell
// ---------------------------------------------------------------------------

describe('formatCell', () => {
	it('returns empty string for null', () => {
		expect(formatCell(null)).toBe('');
	});

	it('returns empty string for undefined', () => {
		expect(formatCell(undefined)).toBe('');
	});

	it('passes string through as-is', () => {
		expect(formatCell('hello')).toBe('hello');
	});

	it('converts number to string', () => {
		expect(formatCell(42)).toBe('42');
	});

	it('converts boolean to string', () => {
		expect(formatCell(true)).toBe('true');
		expect(formatCell(false)).toBe('false');
	});

	it('JSON.stringifies plain objects', () => {
		expect(formatCell({ a: 1 })).toBe('{"a":1}');
	});

	it('JSON.stringifies arrays', () => {
		expect(formatCell([1, 2, 3])).toBe('[1,2,3]');
	});
});

// ---------------------------------------------------------------------------
// chooseRenderer
// ---------------------------------------------------------------------------

describe('chooseRenderer — error always wins', () => {
	it('returns error when isError=true with no hint', () => {
		expect(chooseRenderer(undefined, null, true)).toBe('error');
	});

	it('returns error when isError=true even with table hint', () => {
		expect(chooseRenderer('table', [{ a: 1 }], true)).toBe('error');
	});

	it('returns error when isError=true even with card hint', () => {
		expect(chooseRenderer('card', { a: 1 }, true)).toBe('error');
	});

	it('returns error when isError=true with array-of-objects result', () => {
		expect(chooseRenderer(undefined, [{ x: 1 }], true)).toBe('error');
	});
});

describe('chooseRenderer — explicit render hints (isError=false)', () => {
	it('returns table for explicit table hint with a plain scalar result', () => {
		expect(chooseRenderer('table', 'whatever', false)).toBe('table');
	});

	it('returns card for explicit card hint with a scalar result', () => {
		expect(chooseRenderer('card', 'whatever', false)).toBe('card');
	});

	it('returns summary for explicit summary hint', () => {
		expect(chooseRenderer('summary', 'some text', false)).toBe('summary');
	});
});

describe('chooseRenderer — data-shape inference (no hint)', () => {
	it('infers table for a non-empty array of plain objects', () => {
		expect(chooseRenderer(undefined, [{ id: 1, name: 'Alice' }], false)).toBe('table');
	});

	it('infers table for multiple rows', () => {
		expect(
			chooseRenderer(undefined, [{ a: 1 }, { b: 2 }, { a: 3, b: 4 }], false)
		).toBe('table');
	});

	it('does NOT infer table for an empty array', () => {
		expect(chooseRenderer(undefined, [], false)).toBe('summary');
	});

	it('does NOT infer table for an array containing non-objects', () => {
		expect(chooseRenderer(undefined, [1, 2, 3], false)).toBe('summary');
	});

	it('does NOT infer table for an array of mixed shapes (object + primitive)', () => {
		expect(chooseRenderer(undefined, [{ a: 1 }, 'oops'], false)).toBe('summary');
	});

	it('infers card for a plain object', () => {
		expect(chooseRenderer(undefined, { total: 42, currency: 'AUD' }, false)).toBe('card');
	});

	it('does NOT infer card for null', () => {
		expect(chooseRenderer(undefined, null, false)).toBe('summary');
	});

	it('does NOT infer card for an array (even a single-element one)', () => {
		// A single-element array of objects becomes table, not card.
		expect(chooseRenderer(undefined, [{ x: 1 }], false)).toBe('table');
	});

	it('returns summary for a string', () => {
		expect(chooseRenderer(undefined, 'hello', false)).toBe('summary');
	});

	it('returns summary for a number', () => {
		expect(chooseRenderer(undefined, 99, false)).toBe('summary');
	});

	it('returns summary for a boolean', () => {
		expect(chooseRenderer(undefined, true, false)).toBe('summary');
	});

	it('returns summary for undefined result', () => {
		expect(chooseRenderer(undefined, undefined, false)).toBe('summary');
	});
});

// ---------------------------------------------------------------------------
// tableColumns
// ---------------------------------------------------------------------------

describe('tableColumns', () => {
	it('returns an empty array for an empty input', () => {
		expect(tableColumns([])).toEqual([]);
	});

	it('returns the keys of a single row', () => {
		expect(tableColumns([{ id: 1, name: 'Alice', amount: 10.5 }])).toEqual([
			'id',
			'name',
			'amount'
		]);
	});

	it('preserves first-seen key order across rows', () => {
		// Second row introduces 'b'; 'a' was already seen.
		expect(tableColumns([{ a: 1 }, { b: 2, a: 3 }])).toEqual(['a', 'b']);
	});

	it('deduplicates keys that appear in every row', () => {
		const rows = [
			{ x: 1, y: 2 },
			{ x: 3, y: 4 },
			{ x: 5, y: 6 }
		];
		expect(tableColumns(rows)).toEqual(['x', 'y']);
	});

	it('unions keys from rows with different shapes', () => {
		const rows = [{ a: 1 }, { b: 2 }, { a: 3, c: 4 }];
		expect(tableColumns(rows)).toEqual(['a', 'b', 'c']);
	});
});
