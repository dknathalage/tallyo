/**
 * Rune-based store for the tenant's sessions. Holds the full session list (powering
 * the sessions table, pipeline counts and calendar) plus the derived to-record and
 * suggestion prompts. Refetches on the SSE "session" entity invalidation (the
 * backend broadcasts entity "session" on every session mutation, and "invoice" when
 * a draft cascades session statuses).
 *
 * Follows collection.svelte.ts conventions: reactive list + loading + error
 * getters, a load(), and a one-shot subscribe.
 */

import { onEntity } from '$lib/realtime/events';
import { listAll, suggestions as fetchSuggestions, toRecord as fetchToRecord } from '$lib/api/sessions';
import type { Session, SessionSuggestion } from '$lib/api/types';

export function createSessionsStore() {
	let items = $state<Session[]>([]);
	let toRecordItems = $state<Session[]>([]);
	let suggestionItems = $state<SessionSuggestion[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let registered = false;

	async function load(): Promise<void> {
		loading = true;
		error = null;
		try {
			const [all, rec, sug] = await Promise.all([
				listAll(),
				fetchToRecord(),
				fetchSuggestions()
			]);
			items = all;
			toRecordItems = rec;
			suggestionItems = sug;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	/** Subscribe to the "session" + "invoice" SSE invalidations once (browser only). */
	function ensureSubscribed(): void {
		if (registered) return;
		registered = true;
		onEntity('session', () => {
			void load();
		});
		onEntity('invoice', () => {
			void load();
		});
	}

	return {
		get items() {
			return items;
		},
		get toRecord() {
			return toRecordItems;
		},
		get suggestions() {
			return suggestionItems;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		load,
		ensureSubscribed
	};
}

export const sessions = createSessionsStore();
