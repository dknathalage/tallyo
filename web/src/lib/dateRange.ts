/**
 * Relative date-range presets resolved to absolute YYYY-MM-DD strings in the
 * browser's local time zone. Used by the notes journal "Create invoice from
 * notes" action and the journal range filter. Pure functions, no I/O.
 *
 * Weeks are Monday-based (ISO), matching common AU/NDIS reporting conventions.
 */

export type RangePreset =
	| 'this-week'
	| 'last-week'
	| 'last-7'
	| 'last-30'
	| 'this-month'
	| 'custom';

export interface DateRange {
	from: string;
	to: string;
}

/** Format a Date as a local-time YYYY-MM-DD (never UTC, to avoid off-by-one). */
export function toISODate(d: Date): string {
	const y = d.getFullYear();
	const m = String(d.getMonth() + 1).padStart(2, '0');
	const day = String(d.getDate()).padStart(2, '0');
	return `${y}-${m}-${day}`;
}

/** A new Date offset by `days` from `base` (does not mutate `base`). */
function addDays(base: Date, days: number): Date {
	const d = new Date(base.getFullYear(), base.getMonth(), base.getDate());
	d.setDate(d.getDate() + days);
	return d;
}

/** Days since Monday for the given date (Mon=0 … Sun=6). */
function daysSinceMonday(d: Date): number {
	// getDay(): Sun=0 … Sat=6. Map so Monday is the start of the week.
	const dow = d.getDay();
	return (dow + 6) % 7;
}

/**
 * Resolve a non-custom preset to an absolute inclusive [from, to] range, anchored
 * on `today` (defaults to now). Throws for 'custom' — callers supply those dates.
 */
export function resolvePreset(preset: RangePreset, today: Date = new Date()): DateRange {
	const base = new Date(today.getFullYear(), today.getMonth(), today.getDate());
	switch (preset) {
		case 'this-week': {
			const start = addDays(base, -daysSinceMonday(base));
			return { from: toISODate(start), to: toISODate(addDays(start, 6)) };
		}
		case 'last-week': {
			const thisStart = addDays(base, -daysSinceMonday(base));
			const lastStart = addDays(thisStart, -7);
			return { from: toISODate(lastStart), to: toISODate(addDays(lastStart, 6)) };
		}
		case 'last-7':
			return { from: toISODate(addDays(base, -6)), to: toISODate(base) };
		case 'last-30':
			return { from: toISODate(addDays(base, -29)), to: toISODate(base) };
		case 'this-month': {
			const start = new Date(base.getFullYear(), base.getMonth(), 1);
			const end = new Date(base.getFullYear(), base.getMonth() + 1, 0);
			return { from: toISODate(start), to: toISODate(end) };
		}
		case 'custom':
			throw new Error('resolvePreset: custom range has no preset resolution');
		default:
			throw new Error(`resolvePreset: unknown preset ${preset as string}`);
	}
}

/** Human label for each preset (for the picker UI). */
export const PRESET_LABELS: Record<RangePreset, string> = {
	'this-week': 'This week',
	'last-week': 'Last week',
	'last-7': 'Last 7 days',
	'last-30': 'Last 30 days',
	'this-month': 'This month',
	custom: 'Custom range'
};
