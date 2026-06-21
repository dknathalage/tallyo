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

/** List every shift for the current tenant (powers the shifts table). Returns []. */
export async function listAll(): Promise<Shift[]> {
	return (await apiGet<Shift[]>(tenantPath('shifts'))) ?? [];
}

/**
 * List one participant's shifts, optionally bounded to a [from, to] inclusive
 * service-date range (YYYY-MM-DD) and/or a single lifecycle status. Returns [].
 */
export async function listForParticipant(
	participantId: number,
	from?: string,
	to?: string,
	status?: ShiftStatus
): Promise<Shift[]> {
	if (!Number.isInteger(participantId) || participantId <= 0) {
		throw new Error(
			`shifts.listForParticipant: participantId must be positive, got ${participantId}`
		);
	}
	const params = new URLSearchParams();
	if (from !== undefined && from.length > 0) params.set('from', from);
	if (to !== undefined && to.length > 0) params.set('to', to);
	if (status !== undefined && status.length > 0) params.set('status', status);
	const qs = params.toString();
	const base = tenantPath(`participants/${participantId}/shifts`);
	const path = qs.length > 0 ? `${base}?${qs}` : base;
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

/** Create a shift. Returns the persisted Shift (201). */
export async function create(input: ShiftInput): Promise<Shift> {
	if (!Number.isInteger(input.participantId) || input.participantId <= 0) {
		throw new Error('shifts.create: input.participantId must be positive');
	}
	if (input.serviceDate.length === 0) {
		throw new Error('shifts.create: input.serviceDate is required');
	}
	return must(await apiPost<Shift>(tenantPath('shifts'), input), 'shifts create');
}

/** Update a shift by id. Returns the updated Shift. */
export async function update(id: number, input: ShiftInput): Promise<Shift> {
	if (!Number.isInteger(id) || id <= 0) {
		throw new Error(`shifts.update: id must be positive, got ${id}`);
	}
	return must(await apiPut<Shift>(tenantPath(`shifts/${id}`), input), 'shifts update');
}

/** Delete a shift by id (204). */
export async function remove(id: number): Promise<void> {
	if (!Number.isInteger(id) || id <= 0) {
		throw new Error(`shifts.remove: id must be positive, got ${id}`);
	}
	await apiDelete<void>(tenantPath(`shifts/${id}`));
}

/** Advance a shift's lifecycle status (204). */
export async function setStatus(id: number, status: ShiftStatus): Promise<void> {
	if (!Number.isInteger(id) || id <= 0) {
		throw new Error(`shifts.setStatus: id must be positive, got ${id}`);
	}
	if (status.length === 0) {
		throw new Error('shifts.setStatus: status is required');
	}
	await apiPost<void>(tenantPath(`shifts/${id}/status`), { status });
}

/**
 * Extract recorded shifts from a free-text timesheet for one participant (AI).
 * Returns the created shifts (201).
 */
export async function importShifts(participantId: number, text: string): Promise<Shift[]> {
	if (!Number.isInteger(participantId) || participantId <= 0) {
		throw new Error(`shifts.import: participantId must be positive, got ${participantId}`);
	}
	if (text.trim().length === 0) {
		throw new Error('shifts.import: text is required');
	}
	return (await apiPost<Shift[]>(tenantPath('shifts/import'), { participantId, text })) ?? [];
}

/**
 * Draft one invoice from a set of recorded shifts (deterministic link of their
 * already-priced items — no AI). All shifts must share one participant and each
 * must carry at least one item. Returns the created Invoice (201).
 */
export async function draftFromShifts(shiftIds: number[]): Promise<Invoice> {
	if (shiftIds.length === 0) {
		throw new Error('shifts.draftFromShifts: shiftIds is required');
	}
	return must(
		await apiPost<Invoice>(tenantPath('invoices/draft-from-shifts'), { shiftIds }),
		'shifts draftFromShifts'
	);
}

/** List a shift's line items (billed + unbilled), [] when none. */
export async function listItems(shiftId: number): Promise<LineItem[]> {
	if (!Number.isInteger(shiftId) || shiftId <= 0) {
		throw new Error(`shifts.listItems: shiftId must be positive, got ${shiftId}`);
	}
	return (await apiGet<LineItem[]>(tenantPath(`shifts/${shiftId}/items`))) ?? [];
}

/** Add one line item to a shift (server prices it). Returns the item (201). */
export async function addItem(shiftId: number, input: LineItemInput): Promise<LineItem> {
	if (!Number.isInteger(shiftId) || shiftId <= 0) {
		throw new Error(`shifts.addItem: shiftId must be positive, got ${shiftId}`);
	}
	return must(await apiPost<LineItem>(tenantPath(`shifts/${shiftId}/items`), input), 'shifts addItem');
}

/** Update one unbilled item on a shift. Returns the item. */
export async function updateItem(
	shiftId: number,
	itemId: number,
	input: LineItemInput
): Promise<LineItem> {
	if (!Number.isInteger(shiftId) || shiftId <= 0) {
		throw new Error(`shifts.updateItem: shiftId must be positive, got ${shiftId}`);
	}
	if (!Number.isInteger(itemId) || itemId <= 0) {
		throw new Error(`shifts.updateItem: itemId must be positive, got ${itemId}`);
	}
	return must(
		await apiPatch<LineItem>(tenantPath(`shifts/${shiftId}/items/${itemId}`), input),
		'shifts updateItem'
	);
}

/** Delete one unbilled item from a shift (204). */
export async function deleteItem(shiftId: number, itemId: number): Promise<void> {
	if (!Number.isInteger(shiftId) || shiftId <= 0) {
		throw new Error(`shifts.deleteItem: shiftId must be positive, got ${shiftId}`);
	}
	if (!Number.isInteger(itemId) || itemId <= 0) {
		throw new Error(`shifts.deleteItem: itemId must be positive, got ${itemId}`);
	}
	await apiDelete<void>(tenantPath(`shifts/${shiftId}/items/${itemId}`));
}

/**
 * Divide a shift's note into priced catalogue line items via AI (idempotent —
 * replaces the shift's unbilled items). Returns the shift's items.
 */
export async function divideShift(shiftId: number): Promise<LineItem[]> {
	if (!Number.isInteger(shiftId) || shiftId <= 0) {
		throw new Error(`shifts.divideShift: shiftId must be positive, got ${shiftId}`);
	}
	return (await apiPost<LineItem[]>(tenantPath(`shifts/${shiftId}/divide`), {})) ?? [];
}
