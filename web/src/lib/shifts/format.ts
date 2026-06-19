/**
 * Pure display + date helpers shared by the shift views (home, calendar,
 * participant profile). No I/O. Dates are YYYY-MM-DD strings in the browser's
 * local interpretation; we avoid `new Date(string)` parsing pitfalls by working
 * on the string parts directly where possible.
 */

import type { ShiftStatus } from '$lib/api/types';

const MONTHS = [
	'Jan',
	'Feb',
	'Mar',
	'Apr',
	'May',
	'Jun',
	'Jul',
	'Aug',
	'Sep',
	'Oct',
	'Nov',
	'Dec'
];

const DOW = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

/** Local-time today as YYYY-MM-DD. */
export function todayISO(): string {
	const d = new Date();
	const y = d.getFullYear();
	const m = String(d.getMonth() + 1).padStart(2, '0');
	const day = String(d.getDate()).padStart(2, '0');
	return `${y}-${m}-${day}`;
}

/** "12 Jun" — short day + month from a YYYY-MM-DD string. */
export function shortDate(iso: string): string {
	if (typeof iso !== 'string' || iso.length < 10) return iso;
	const month = Number(iso.slice(5, 7));
	const day = Number(iso.slice(8, 10));
	if (!Number.isFinite(month) || !Number.isFinite(day)) return iso;
	return `${day} ${MONTHS[month - 1] ?? ''}`.trim();
}

/** "Mon 12 Jun" — day-of-week + short date. */
export function dowDate(iso: string): string {
	if (typeof iso !== 'string' || iso.length < 10) return iso;
	// Construct from parts in local time (no TZ shift for date-only strings).
	const y = Number(iso.slice(0, 4));
	const m = Number(iso.slice(5, 7));
	const d = Number(iso.slice(8, 10));
	if (!Number.isFinite(y) || !Number.isFinite(m) || !Number.isFinite(d)) return iso;
	const dow = DOW[new Date(y, m - 1, d).getDay()] ?? '';
	return `${dow} ${shortDate(iso)}`.trim();
}

const STATUS_LABELS: Record<ShiftStatus, string> = {
	scheduled: 'Scheduled',
	recorded: 'Recorded',
	drafted: 'Drafted',
	sent: 'Sent',
	paid: 'Paid'
};

export function statusLabel(status: string): string {
	return STATUS_LABELS[status as ShiftStatus] ?? status;
}

/** Tailwind badge classes per lifecycle status (mirrors the prototype palette). */
export function statusBadgeClass(status: string): string {
	switch (status) {
		case 'scheduled':
			return 'bg-amber-50 text-amber-700 ring-1 ring-amber-200';
		case 'recorded':
			return 'bg-blue-50 text-blue-700 ring-1 ring-blue-200';
		case 'drafted':
			return 'bg-slate-50 text-slate-700 ring-1 ring-slate-200';
		case 'sent':
			return 'bg-teal-50 text-teal-700 ring-1 ring-teal-200';
		case 'paid':
			return 'bg-green-50 text-green-700 ring-1 ring-green-200';
		default:
			return 'bg-gray-100 text-gray-700';
	}
}

/** Calendar event chip classes per status. */
export function eventClass(status: string): string {
	switch (status) {
		case 'scheduled':
			return 'border border-dashed border-amber-400 bg-amber-50 text-amber-700';
		case 'recorded':
			return 'border border-blue-200 bg-blue-50 text-blue-700';
		case 'drafted':
			return 'border border-slate-200 bg-slate-50 text-slate-700';
		case 'sent':
			return 'border border-teal-200 bg-teal-50 text-teal-700';
		case 'paid':
			return 'border border-green-200 bg-green-50 text-green-700';
		default:
			return 'border border-gray-200 bg-gray-50 text-gray-700';
	}
}

/** Hours between two HH:MM times, rounded to 2dp; 0 when either is blank. */
export function hoursBetween(start: string, end: string): number {
	if (!start || !end) return 0;
	const [ah, am] = start.split(':').map(Number);
	const [bh, bm] = end.split(':').map(Number);
	if (![ah, am, bh, bm].every(Number.isFinite)) return 0;
	const mins = bh * 60 + bm - (ah * 60 + am);
	return Math.round((mins / 60) * 100) / 100;
}
