import { apiPost, tenantPath } from './client';
import type { LineItem } from './types';

// AI "Smarts" — one-shot, server-gated LLM helpers (gather → propose → apply).
// Every endpoint 503s when AI is off; the SPA hides the affordances via
// `features.smarts`. These results are backseat suggestions: always editable by
// the user before they take effect, never auto-committed.
//
// apiPost returns Promise<T | null> (null on a 401-redirect). This module unwraps
// that contract like sessions.ts: a null result throws (a 401 already redirected
// to /login, so a null here is a genuine error).
function must<T>(v: T | null, what: string): T {
	if (v === null) throw new Error(`${what}: no data`);
	return v;
}

/** Guard: an id/uuid path segment must be a non-empty string. */
function requireId(value: string, what: string): void {
	if (typeof value !== 'string' || value.length === 0) {
		throw new Error(`${what}: id must be a non-empty string`);
	}
}

/** A drafted reminder email (subject + body), editable before sending. */
export interface FollowUp {
	subject: string;
	body: string;
}

/**
 * Draft a fresh blank invoice for one client (AI seeds it from the client's
 * recent activity). The client is addressed by uuid. Returns the new draft
 * invoice's uuid so the caller can navigate to it.
 */
export async function draftInvoice(clientId: string): Promise<string> {
	requireId(clientId, 'smarts.draftInvoice');
	const got = must(
		await apiPost<{ id: string }>(tenantPath('smarts/draft-invoice'), { clientId }),
		'smarts draftInvoice'
	);
	return got.id;
}

/**
 * Suggest catalogue-priced line items from a free-text note for a service date
 * (YYYY-MM-DD). Returns the same LineItem shape session division produced, ready
 * to append into a billing-document editor. Returns [] when nothing is proposed.
 */
export async function suggestLines(note: string, serviceDate: string): Promise<LineItem[]> {
	if (typeof note !== 'string' || note.length === 0) {
		throw new Error('smarts.suggestLines: note is required');
	}
	if (typeof serviceDate !== 'string' || serviceDate.length === 0) {
		throw new Error('smarts.suggestLines: serviceDate is required');
	}
	return (
		(await apiPost<LineItem[]>(tenantPath('smarts/suggest-lines'), { note, serviceDate })) ?? []
	);
}

/**
 * Draft a follow-up reminder email for an (overdue) invoice, addressed by uuid.
 * Returns an editable subject + body.
 */
export async function draftFollowUp(invoiceId: string): Promise<FollowUp> {
	requireId(invoiceId, 'smarts.draftFollowUp');
	return must(
		await apiPost<FollowUp>(tenantPath('smarts/follow-up'), { invoiceId }),
		'smarts draftFollowUp'
	);
}

/**
 * Propose a header → target-field mapping for a price-list import from the
 * detected headers and a sample of rows. Returns a sourceHeader → targetField
 * map the import wizard pre-fills (and the user can adjust). Returns {} when
 * nothing maps.
 */
export async function mapImport(
	headers: string[],
	rows: Record<string, string>[]
): Promise<Record<string, string>> {
	if (!Array.isArray(headers) || headers.length === 0) {
		throw new Error('smarts.mapImport: headers is required');
	}
	const got = await apiPost<{ mappings: Record<string, string> }>(
		tenantPath('smarts/map-import'),
		{ headers, rows }
	);
	return got?.mappings ?? {};
}
