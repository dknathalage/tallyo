/**
 * Rune-based store for the tenant's shifts. Holds the full shift list (powering
 * the shifts table, pipeline counts and calendar) plus the derived to-record and
 * suggestion prompts. Refetches on the SSE "shift" entity invalidation (the
 * backend broadcasts entity "shift" on every shift mutation, and "invoice" when
 * a draft cascades shift statuses).
 *
 * Follows collection.svelte.ts conventions: reactive list + loading + error
 * getters, a load(), and a one-shot subscribe.
 */

import { onEntity } from '$lib/realtime/events';
import { listAll, suggestions as fetchSuggestions, toRecord as fetchToRecord } from '$lib/api/shifts';
import type { Shift, ShiftSuggestion } from '$lib/api/types';

export function createShiftsStore() {
	let items = $state<Shift[]>([]);
	let toRecordItems = $state<Shift[]>([]);
	let suggestionItems = $state<ShiftSuggestion[]>([]);
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

	/** Subscribe to the "shift" + "invoice" SSE invalidations once (browser only). */
	function ensureSubscribed(): void {
		if (registered) return;
		registered = true;
		onEntity('shift', () => {
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

export const shifts = createShiftsStore();
