/**
 * Singleton SSE client for /api/events.
 *
 * The Go backend emits JSON frames {entity, id, action}. We fan these out to
 * per-entity listeners. On (re)connect ("open"), every registered listener is
 * fired so stores can resync after a dropped connection.
 */

interface EventFrame {
	entity: string;
	id: string;
	action: string;
}

type Listener = () => void;

import { tenantPath } from '$lib/api/client';

const listeners = new Map<string, Set<Listener>>();
let source: EventSource | null = null;

function fireAll(): void {
	// Bounded by the number of registered entities; safe to iterate fully.
	for (const set of listeners.values()) {
		for (const cb of set) {
			cb();
		}
	}
}

function fireEntity(entity: string): void {
	const set = listeners.get(entity);
	if (set === undefined) return;
	for (const cb of set) {
		cb();
	}
}

function ensureOpen(): void {
	if (typeof window === 'undefined') return;
	if (source !== null) return;

	// A pre-tenant call (e.g. lazy onEntity before the layout set the active
	// tenant) must no-op rather than crash: tenantPath throws with no tenant.
	let url: string;
	try {
		url = tenantPath('events');
	} catch {
		return;
	}

	const es = new EventSource(url, { withCredentials: true });
	source = es;

	es.addEventListener('open', () => {
		// Covers initial connect and browser auto-reconnect: resync everything.
		fireAll();
	});

	es.addEventListener('message', (event: MessageEvent) => {
		if (typeof event.data !== 'string' || event.data.length === 0) return;
		let frame: EventFrame;
		try {
			frame = JSON.parse(event.data) as EventFrame;
		} catch {
			// Malformed frame; ignore rather than crash the stream.
			return;
		}
		if (typeof frame.entity !== 'string') return;
		fireEntity(frame.entity);
	});

	// On error we intentionally do NOT close: the browser auto-reconnects and
	// the next "open" triggers a resync.
}

/** Open the SSE stream for the active tenant (no-op if already open). */
export function openEvents(): void {
	ensureOpen();
}

/** Close the SSE stream (e.g. on tenant switch, before reopening). */
export function closeEvents(): void {
	if (source !== null) {
		source.close();
		source = null;
	}
}

/**
 * Register a callback for an entity's change events. Returns an unsubscribe.
 * Lazily opens the EventSource on first use (browser only).
 */
export function onEntity(entity: string, cb: Listener): () => void {
	if (typeof entity !== 'string' || entity.length === 0) {
		throw new Error('onEntity: entity must be a non-empty string');
	}
	if (typeof cb !== 'function') {
		throw new Error('onEntity: cb must be a function');
	}

	let set = listeners.get(entity);
	if (set === undefined) {
		set = new Set();
		listeners.set(entity, set);
	}
	set.add(cb);

	ensureOpen();

	return () => {
		const current = listeners.get(entity);
		if (current === undefined) return;
		current.delete(cb);
		if (current.size === 0) {
			listeners.delete(entity);
		}
	};
}
