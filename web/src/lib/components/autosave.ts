export type SaveState = 'idle' | 'saving' | 'saved' | 'error';

export interface AutosaveOptions<T, R extends { id: number }> {
	/** Existing record id when editing; null/undefined for a brand-new record. */
	initialId?: number | null;
	/** Persist a brand-new record; resolves to the created entity (carrying its id). */
	create: (payload: T) => Promise<R>;
	/** Persist an update to an already-created record. */
	update: (id: number, payload: T) => Promise<R>;
	/** Debounce window before a scheduled flush fires (ms). */
	delay?: number;
	/** Called on every state transition — drive the status line from this. */
	onState?: (state: SaveState) => void;
	/** Called once with the new id the first time a `new` record is created. */
	onCreated?: (id: number) => void;
}

export interface Autosave<T> {
	/** Debounced save of the latest payload (newer payloads supersede older). */
	schedule: (payload: T) => void;
	/** Flush a pending payload immediately. Safe to call fire-and-forget on teardown. */
	flush: () => void;
	/** Re-run the last failed payload (status-line retry button). */
	retry: () => void;
	/** Cancel the debounce timer, flushing any pending edit best-effort. */
	dispose: () => void;
}

/**
 * Debounced single-in-flight save machine. The FIRST successful save of a brand
 * new record calls `create` and captures the returned id; every later save (and
 * any edit that arrives while a save is in flight) calls `update` with that id.
 * Errors are surfaced via `onState('error')` and the failed payload is held for
 * `retry()` — never auto-retried (so a persistent failure can't spin).
 */
export function createAutosave<T, R extends { id: number }>(
	opts: AutosaveOptions<T, R>
): Autosave<T> {
	const delay = opts.delay ?? 400;
	let id: number | null = opts.initialId ?? null; // existing id, or null until first create
	let timer: ReturnType<typeof setTimeout> | null = null;
	let saving = false;
	let pending: T | null = null; // latest payload awaiting a flush
	let dirtyDuringSave = false; // an edit arrived while a save was in flight
	let lastFailed: T | null = null;

	function schedule(payload: T): void {
		pending = payload;
		if (timer) clearTimeout(timer);
		timer = setTimeout(flush, delay);
	}

	function flush(): void {
		if (timer) {
			clearTimeout(timer);
			timer = null;
		}
		if (pending === null) return;
		if (saving) {
			dirtyDuringSave = true;
			return;
		}
		const payload = pending;
		pending = null;
		saving = true;
		opts.onState?.('saving');
		const op = id === null ? opts.create(payload) : opts.update(id, payload);
		void op
			.then((row) => {
				if (id === null) {
					id = row.id;
					opts.onCreated?.(row.id);
				}
				opts.onState?.('saved');
			})
			.catch(() => {
				lastFailed = payload;
				opts.onState?.('error');
			})
			.finally(() => {
				saving = false;
				if (dirtyDuringSave) {
					dirtyDuringSave = false;
					flush();
				}
			});
	}

	function retry(): void {
		if (lastFailed === null) return;
		pending = lastFailed;
		lastFailed = null;
		flush();
	}

	function dispose(): void {
		if (timer) {
			clearTimeout(timer);
			timer = null;
			flush(); // best-effort; never blocks
		}
	}

	return { schedule, flush, retry, dispose };
}
