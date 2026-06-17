import { apiGet, apiPost, apiPut, apiDelete } from './client';
import type { Note, NoteInput } from './types';

// NOTE: apiGet/apiPost/apiPut return Promise<T | null> (null on 401-redirect or
// 204). This module unwraps the contract the way crud.ts does: list falls back
// to [], create/update throw on a null result (a 401 already redirected to
// /login, so a null here is a genuine error). Keeps a non-null surface.
function must<T>(v: T | null, what: string): T {
	if (v === null) throw new Error(`${what}: no data`);
	return v;
}

/**
 * List notes for one participant, optionally bounded to a [from, to] date range
 * (inclusive, YYYY-MM-DD). Returns [] when empty. Both range params are optional.
 */
export async function listForParticipant(
	participantId: number,
	from?: string,
	to?: string
): Promise<Note[]> {
	if (!Number.isInteger(participantId) || participantId <= 0) {
		throw new Error(`notes.listForParticipant: participantId must be positive, got ${participantId}`);
	}
	const params = new URLSearchParams();
	if (from !== undefined && from.length > 0) params.set('from', from);
	if (to !== undefined && to.length > 0) params.set('to', to);
	const qs = params.toString();
	const base = `/api/participants/${participantId}/notes`;
	const path = qs.length > 0 ? `${base}?${qs}` : base;
	return (await apiGet<Note[]>(path)) ?? [];
}

/** Create a note. Returns the persisted Note (201). */
export async function create(input: NoteInput): Promise<Note> {
	if (!Number.isInteger(input.participantId) || input.participantId <= 0) {
		throw new Error('notes.create: input.participantId must be positive');
	}
	return must(await apiPost<Note>('/api/notes', input), 'notes create');
}

/** Update a note by id. Returns the updated Note. */
export async function update(id: number, input: NoteInput): Promise<Note> {
	if (!Number.isInteger(id) || id <= 0) {
		throw new Error(`notes.update: id must be positive, got ${id}`);
	}
	return must(await apiPut<Note>(`/api/notes/${id}`, input), 'notes update');
}

/** Delete a note by id (204). */
export async function remove(id: number): Promise<void> {
	if (!Number.isInteger(id) || id <= 0) {
		throw new Error(`notes.remove: id must be positive, got ${id}`);
	}
	await apiDelete<void>(`/api/notes/${id}`);
}

/** Attach the given notes to an invoice as billed (204). */
export async function bill(invoiceId: number, noteIds: number[]): Promise<void> {
	if (!Number.isInteger(invoiceId) || invoiceId <= 0) {
		throw new Error(`notes.bill: invoiceId must be positive, got ${invoiceId}`);
	}
	if (!Array.isArray(noteIds) || noteIds.length === 0) {
		throw new Error('notes.bill: noteIds must be a non-empty array');
	}
	await apiPost<void>('/api/notes/bill', { invoiceId, noteIds });
}
