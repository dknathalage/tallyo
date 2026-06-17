/**
 * Rune-based store for a single participant's notes journal, scoped to an
 * optional [from, to] date range. Unlike the generic collection store, notes are
 * fetched per-participant with range params, so this store owns that query shape.
 *
 * Refetches on the SSE "note" entity invalidation (the backend broadcasts entity
 * "note" on every note mutation). Follows collection.svelte.ts conventions:
 * reactive list + loading + error getters, a load(), and a one-shot subscribe.
 */

import { onEntity } from '$lib/realtime/events';
import { listForParticipant } from '$lib/api/notes';
import type { Note } from '$lib/api/types';

export function createNotesStore() {
	let participantId = $state<number | null>(null);
	let from = $state<string | null>(null);
	let to = $state<string | null>(null);
	let items = $state<Note[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let registered = false;

	async function load(): Promise<void> {
		const pid = participantId;
		if (pid === null) {
			items = [];
			return;
		}
		loading = true;
		error = null;
		try {
			items = await listForParticipant(pid, from ?? undefined, to ?? undefined);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	/**
	 * Point the store at a participant and (optionally) a date range, then load.
	 * Reusing the store for a new scope just calls this again.
	 */
	function setScope(pid: number, rangeFrom?: string | null, rangeTo?: string | null): void {
		if (!Number.isInteger(pid) || pid <= 0) {
			throw new Error(`notes setScope: pid must be a positive integer, got ${pid}`);
		}
		participantId = pid;
		from = rangeFrom ?? null;
		to = rangeTo ?? null;
		void load();
	}

	/** Subscribe to the "note" SSE invalidation exactly once (browser only). */
	function ensureSubscribed(): void {
		if (registered) return;
		registered = true;
		onEntity('note', () => {
			void load();
		});
	}

	return {
		get items() {
			return items;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		get participantId() {
			return participantId;
		},
		setScope,
		load,
		ensureSubscribed
	};
}
