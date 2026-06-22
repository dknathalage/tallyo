import { apiGet, apiPost, apiPut, apiPatch, apiDelete, tenantPath } from './client';
import type {
	Shift,
	ShiftInput,
	ShiftStatus,
	ShiftSuggestion,
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

/** List every shift for the current tenant (powers the shifts table). Returns []. */
export async function listAll(): Promise<Shift[]> {
	return (await apiGet<Shift[]>(tenantPath('shifts'))) ?? [];
}

/**
 * List one participant's shifts, optionally bounded to a [from, to] inclusive
 * service-date range (YYYY-MM-DD) and/or a single lifecycle status. Returns [].
 * The participant is addressed by its uuid via the `?participant=` filter.
 */
export async function listForParticipant(
	participantId: string,
	from?: string,
	to?: string,
	status?: ShiftStatus
): Promise<Shift[]> {
	requireId(participantId, 'shifts.listForParticipant');
	const params = new URLSearchParams();
	params.set('participant', participantId);
	if (from !== undefined && from.length > 0) params.set('from', from);
	if (to !== undefined && to.length > 0) params.set('to', to);
	if (status !== undefined && status.length > 0) params.set('status', status);
	const path = `${tenantPath('shifts')}?${params.toString()}`;
	return (await apiGet<Shift[]>(path)) ?? [];
}

/** Recorded-but-unbilled shift clusters, one per participant. Returns []. */
export async function suggestions(): Promise<ShiftSuggestion[]> {
	return (await apiGet<ShiftSuggestion[]>(tenantPath('shifts/suggestions'))) ?? [];
}

/** Scheduled shifts still awaiting a record (overdue / today / upcoming). Returns []. */
export async function toRecord(): Promise<Shift[]> {
	return (await apiGet<Shift[]>(tenantPath('shifts/to-record'))) ?? [];
}

/** Fetch a single shift by uuid. Returns the Shift. */
export async function get(id: string): Promise<Shift> {
	requireId(id, 'shifts.get');
	return must(await apiGet<Shift>(tenantPath(`shifts/${id}`)), 'shifts get');
}

/** Create a shift. Returns the persisted Shift (201). */
export async function create(input: ShiftInput): Promise<Shift> {
	requireId(input.participantId, 'shifts.create');
	if (input.serviceDate.length === 0) {
		throw new Error('shifts.create: input.serviceDate is required');
	}
	return must(await apiPost<Shift>(tenantPath('shifts'), input), 'shifts create');
}

/** Update a shift by uuid. Returns the updated Shift. */
export async function update(id: string, input: ShiftInput): Promise<Shift> {
	requireId(id, 'shifts.update');
	return must(await apiPut<Shift>(tenantPath(`shifts/${id}`), input), 'shifts update');
}

/** Delete a shift by uuid (204). */
export async function remove(id: string): Promise<void> {
	requireId(id, 'shifts.remove');
	await apiDelete<void>(tenantPath(`shifts/${id}`));
}

/** Advance a shift's lifecycle status (204). */
export async function setStatus(id: string, status: ShiftStatus): Promise<void> {
	requireId(id, 'shifts.setStatus');
	if (status.length === 0) {
		throw new Error('shifts.setStatus: status is required');
	}
	await apiPost<void>(tenantPath(`shifts/${id}/status`), { status });
}

/**
 * Extract recorded shifts from a free-text timesheet for one participant (AI).
 * Returns the created shifts (201).
 */
export async function importShifts(participantId: string, text: string): Promise<Shift[]> {
	requireId(participantId, 'shifts.import');
	if (text.trim().length === 0) {
		throw new Error('shifts.import: text is required');
	}
	return (await apiPost<Shift[]>(tenantPath('shifts/import'), { participantId, text })) ?? [];
}

/**
 * Draft one invoice from a set of recorded shifts (deterministic link of their
 * already-priced items — no AI). All shifts must share one participant and each
 * must carry at least one item. Shifts are addressed by uuid. Returns the created
 * Invoice (201).
 */
export async function draftFromShifts(shiftIds: string[]): Promise<Invoice> {
	if (shiftIds.length === 0) {
		throw new Error('shifts.draftFromShifts: shiftIds is required');
	}
	return must(
		await apiPost<Invoice>(tenantPath('invoices/draft-from-shifts'), { shiftIds }),
		'shifts draftFromShifts'
	);
}

/** List a shift's line items (billed + unbilled), [] when none. */
export async function listItems(shiftId: string): Promise<LineItem[]> {
	requireId(shiftId, 'shifts.listItems');
	return (await apiGet<LineItem[]>(tenantPath(`shifts/${shiftId}/items`))) ?? [];
}

/** Add one line item to a shift (server prices it). Returns the item (201). */
export async function addItem(shiftId: string, input: LineItemInput): Promise<LineItem> {
	requireId(shiftId, 'shifts.addItem');
	return must(await apiPost<LineItem>(tenantPath(`shifts/${shiftId}/items`), input), 'shifts addItem');
}

/** Update one unbilled item on a shift. Returns the item. */
export async function updateItem(
	shiftId: string,
	itemId: string,
	input: LineItemInput
): Promise<LineItem> {
	requireId(shiftId, 'shifts.updateItem');
	requireId(itemId, 'shifts.updateItem');
	return must(
		await apiPatch<LineItem>(tenantPath(`shifts/${shiftId}/items/${itemId}`), input),
		'shifts updateItem'
	);
}

/** Delete one unbilled item from a shift (204). */
export async function deleteItem(shiftId: string, itemId: string): Promise<void> {
	requireId(shiftId, 'shifts.deleteItem');
	requireId(itemId, 'shifts.deleteItem');
	await apiDelete<void>(tenantPath(`shifts/${shiftId}/items/${itemId}`));
}

/**
 * Divide a shift's note into priced catalogue line items via AI (idempotent —
 * replaces the shift's unbilled items). Returns the shift's items.
 */
export async function divideShift(shiftId: string): Promise<LineItem[]> {
	requireId(shiftId, 'shifts.divideShift');
	return (await apiPost<LineItem[]>(tenantPath(`shifts/${shiftId}/divide`), {})) ?? [];
}
