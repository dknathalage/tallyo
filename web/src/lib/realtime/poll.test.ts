import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { startPolling } from './poll';

interface ListenerMap {
	[type: string]: Set<(ev?: unknown) => void>;
}

function makeTarget() {
	const listeners: ListenerMap = {};
	let count = 0;
	return {
		addEventListener(type: string, cb: (ev?: unknown) => void): void {
			(listeners[type] ??= new Set()).add(cb);
			count++;
		},
		removeEventListener(type: string, cb: (ev?: unknown) => void): void {
			const set = listeners[type];
			if (set && set.delete(cb)) count--;
		},
		dispatch(type: string): void {
			const set = listeners[type];
			if (set) for (const cb of set) cb();
		},
		get listenerCount(): number {
			return count;
		}
	};
}

describe('startPolling', () => {
	let win: ReturnType<typeof makeTarget>;
	let doc: ReturnType<typeof makeTarget> & { visibilityState: string };

	beforeEach(() => {
		vi.useFakeTimers();
		win = makeTarget();
		const d = makeTarget();
		doc = Object.assign(d, { visibilityState: 'visible' });
		(globalThis as unknown as { window: unknown }).window = win;
		(globalThis as unknown as { document: unknown }).document = doc;
	});

	afterEach(() => {
		vi.useRealTimers();
		delete (globalThis as unknown as { window?: unknown }).window;
		delete (globalThis as unknown as { document?: unknown }).document;
	});

	it('calls refetch immediately', () => {
		const refetch = vi.fn();
		startPolling(refetch);
		expect(refetch).toHaveBeenCalledTimes(1);
	});

	it('calls refetch again after the interval elapses', () => {
		const refetch = vi.fn();
		startPolling(refetch);
		expect(refetch).toHaveBeenCalledTimes(1);
		vi.advanceTimersByTime(30000);
		expect(refetch).toHaveBeenCalledTimes(2);
	});

	it('refetches on window focus', () => {
		const refetch = vi.fn();
		startPolling(refetch);
		expect(refetch).toHaveBeenCalledTimes(1);
		win.dispatch('focus');
		expect(refetch).toHaveBeenCalledTimes(2);
	});

	it('refetches on visibilitychange when visible', () => {
		const refetch = vi.fn();
		startPolling(refetch);
		expect(refetch).toHaveBeenCalledTimes(1);
		doc.visibilityState = 'visible';
		doc.dispatch('visibilitychange');
		expect(refetch).toHaveBeenCalledTimes(2);
	});

	it('does not refetch on visibilitychange when hidden', () => {
		const refetch = vi.fn();
		startPolling(refetch);
		expect(refetch).toHaveBeenCalledTimes(1);
		doc.visibilityState = 'hidden';
		doc.dispatch('visibilitychange');
		expect(refetch).toHaveBeenCalledTimes(1);
	});

	it('stops everything and removes listeners after cleanup', () => {
		const refetch = vi.fn();
		const stop = startPolling(refetch);
		expect(refetch).toHaveBeenCalledTimes(1);
		stop();
		vi.advanceTimersByTime(30000);
		win.dispatch('focus');
		doc.dispatch('visibilitychange');
		expect(refetch).toHaveBeenCalledTimes(1);
		expect(win.listenerCount).toBe(0);
		expect(doc.listenerCount).toBe(0);
	});

	it('is a no-op under SSR (no window/document) and returns a function', () => {
		delete (globalThis as unknown as { window?: unknown }).window;
		delete (globalThis as unknown as { document?: unknown }).document;
		const refetch = vi.fn();
		const stop = startPolling(refetch);
		expect(refetch).not.toHaveBeenCalled();
		expect(typeof stop).toBe('function');
		expect(() => stop()).not.toThrow();
	});

	it('throws when refetch is not a function', () => {
		expect(() => startPolling(undefined as unknown as () => void)).toThrow();
	});
});
