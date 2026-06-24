import { apiGet, apiPost, apiPut, apiPatch, apiDelete, tenantPath } from './client';
import type {
	Session,
	SessionInput,
	SessionStatus,
	SessionSuggestion,
	Invoice,
	LineItem,
	LineItemInput
} from './types';

// NOTE: apiGet/apiPost/apiPut return Promise<T | null> (null on 401-redirect or
// 204). This module unwraps the contract the way crud.ts does: list calls fall
// back to [], create/update/import/draft throw on a null result (a 401 already
// redirected to /login, so a null here is a genuine error). Keeps a non-null
// surface for callers.
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

/** List every session for the current tenant (powers the sessions table). Returns []. */
export async function listAll(): Promise<Session[]> {
	return (await apiGet<Session[]>(tenantPath('sessions'))) ?? [];
}

/**
 * List one client's sessions, optionally bounded to a [from, to] inclusive
 * service-date range (YYYY-MM-DD) and/or a single lifecycle status. Returns [].
 * The client is addressed by its uuid via the `?client=` filter.
 */
export async function listForClient(
	clientId: string,
	from?: string,
	to?: string,
	status?: SessionStatus
): Promise<Session[]> {
	requireId(clientId, 'sessions.listForClient');
	const params = new URLSearchParams();
	params.set('client', clientId);
	if (from !== undefined && from.length > 0) params.set('from', from);
	if (to !== undefined && to.length > 0) params.set('to', to);
	if (status !== undefined && status.length > 0) params.set('status', status);
	const path = `${tenantPath('sessions')}?${params.toString()}`;
	return (await apiGet<Session[]>(path)) ?? [];
}

/** Recorded-but-unbilled session clusters, one per client. Returns []. */
export async function suggestions(): Promise<SessionSuggestion[]> {
	return (await apiGet<SessionSuggestion[]>(tenantPath('sessions/suggestions'))) ?? [];
}

/** Scheduled sessions still awaiting a record (overdue / today / upcoming). Returns []. */
export async function toRecord(): Promise<Session[]> {
	return (await apiGet<Session[]>(tenantPath('sessions/to-record'))) ?? [];
}

/** Fetch a single session by uuid. Returns the Session. */
export async function get(id: string): Promise<Session> {
	requireId(id, 'sessions.get');
	return must(await apiGet<Session>(tenantPath(`sessions/${id}`)), 'sessions get');
}

/** Create a session. Returns the persisted Session (201). */
export async function create(input: SessionInput): Promise<Session> {
	requireId(input.clientId, 'sessions.create');
	if (input.serviceDate.length === 0) {
		throw new Error('sessions.create: input.serviceDate is required');
	}
	return must(await apiPost<Session>(tenantPath('sessions'), input), 'sessions create');
}

/** Update a session by uuid. Returns the updated Session. */
export async function update(id: string, input: SessionInput): Promise<Session> {
	requireId(id, 'sessions.update');
	return must(await apiPut<Session>(tenantPath(`sessions/${id}`), input), 'sessions update');
}

/** Delete a session by uuid (204). */
export async function remove(id: string): Promise<void> {
	requireId(id, 'sessions.remove');
	await apiDelete<void>(tenantPath(`sessions/${id}`));
}

/** Advance a session's lifecycle status (204). */
export async function setStatus(id: string, status: SessionStatus): Promise<void> {
	requireId(id, 'sessions.setStatus');
	if (status.length === 0) {
		throw new Error('sessions.setStatus: status is required');
	}
	await apiPost<void>(tenantPath(`sessions/${id}/status`), { status });
}

/**
 * Draft one invoice from a set of recorded sessions (deterministic link of their
 * already-priced items — no AI). All sessions must share one client and each
 * must carry at least one item. Sessions are addressed by uuid. Returns the created
 * Invoice (201).
 */
export async function draftFromSessions(sessionIds: string[]): Promise<Invoice> {
	if (sessionIds.length === 0) {
		throw new Error('sessions.draftFromSessions: sessionIds is required');
	}
	return must(
		await apiPost<Invoice>(tenantPath('invoices/draft-from-sessions'), { sessionIds }),
		'sessions draftFromSessions'
	);
}

/** List a session's line items (billed + unbilled), [] when none. */
export async function listItems(sessionId: string): Promise<LineItem[]> {
	requireId(sessionId, 'sessions.listItems');
	return (await apiGet<LineItem[]>(tenantPath(`sessions/${sessionId}/items`))) ?? [];
}

/** Add one line item to a session (server prices it). Returns the item (201). */
export async function addItem(sessionId: string, input: LineItemInput): Promise<LineItem> {
	requireId(sessionId, 'sessions.addItem');
	return must(await apiPost<LineItem>(tenantPath(`sessions/${sessionId}/items`), input), 'sessions addItem');
}

/** Update one unbilled item on a session. Returns the item. */
export async function updateItem(
	sessionId: string,
	itemId: string,
	input: LineItemInput
): Promise<LineItem> {
	requireId(sessionId, 'sessions.updateItem');
	requireId(itemId, 'sessions.updateItem');
	return must(
		await apiPatch<LineItem>(tenantPath(`sessions/${sessionId}/items/${itemId}`), input),
		'sessions updateItem'
	);
}

/** Delete one unbilled item from a session (204). */
export async function deleteItem(sessionId: string, itemId: string): Promise<void> {
	requireId(sessionId, 'sessions.deleteItem');
	requireId(itemId, 'sessions.deleteItem');
	await apiDelete<void>(tenantPath(`sessions/${sessionId}/items/${itemId}`));
}
