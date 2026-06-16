/**
 * Pure helper — no Svelte, no side effects.
 * Determines which renderer component to use for a tool result.
 */

export type RendererKind = 'table' | 'card' | 'summary' | 'error';

/**
 * Return true when `value` is a plain (non-null, non-array) object.
 * Arrays, null, Date, etc. are NOT plain objects.
 */
function isPlainObject(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * Return true when `value` is a non-empty array whose every element is a
 * plain object (i.e. a row set suitable for a table renderer).
 */
function isArrayOfObjects(value: unknown): value is Record<string, unknown>[] {
	if (!Array.isArray(value) || value.length === 0) return false;
	for (const item of value) {
		if (!isPlainObject(item)) return false;
	}
	return true;
}

/**
 * Choose the appropriate renderer kind for a tool result.
 *
 * Priority order:
 *   1. isError → 'error' (always wins regardless of hint or data shape)
 *   2. render hint === 'table' OR result is non-empty array of objects → 'table'
 *   3. render hint === 'card'  OR result is a plain object → 'card'
 *   4. Otherwise → 'summary' (scalars, strings, null, empty arrays, …)
 */
export function chooseRenderer(
	render: string | undefined,
	result: unknown,
	isError: boolean
): RendererKind {
	if (isError) return 'error';
	if (render === 'table' || isArrayOfObjects(result)) return 'table';
	if (render === 'card' || isPlainObject(result)) return 'card';
	return 'summary';
}

/**
 * Format a single table/card cell value for display.
 * - null/undefined  → ''
 * - string          → as-is
 * - number/boolean  → String(v)
 * - object/array    → JSON.stringify(v)
 */
export function formatCell(v: unknown): string {
	if (v === null || v === undefined) return '';
	if (typeof v === 'string') return v;
	if (typeof v === 'number' || typeof v === 'boolean') return String(v);
	return JSON.stringify(v);
}

/**
 * Derive an ordered column list from an array of row objects.
 * Keys appear in first-seen order, deduplicated.
 */
export function tableColumns(rows: Record<string, unknown>[]): string[] {
	const seen = new Set<string>();
	const columns: string[] = [];
	for (const row of rows) {
		for (const key of Object.keys(row)) {
			if (!seen.has(key)) {
				seen.add(key);
				columns.push(key);
			}
		}
	}
	return columns;
}
