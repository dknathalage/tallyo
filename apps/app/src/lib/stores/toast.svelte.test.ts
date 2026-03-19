import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { addToast, removeToast, getToasts } from './toast.svelte.js';

beforeEach(() => {
	vi.useFakeTimers();
	// Clear all toasts between tests by removing them
	const toasts = getToasts();
	for (const t of [...toasts]) {
		removeToast(t.id);
	}
});

afterEach(() => {
	vi.useRealTimers();
});

describe('addToast', () => {
	it('adds a toast and returns an id', () => {
		const id = addToast({ message: 'Hello' });
		expect(typeof id).toBe('string');
		expect(id.length).toBeGreaterThan(0);
	});

	it('added toast appears in getToasts()', () => {
		const id = addToast({ message: 'Test message' });
		const toasts = getToasts();
		const found = toasts.find((t) => t.id === id);
		expect(found).toBeDefined();
		expect(found!.message).toBe('Test message');
	});

	it('defaults type to info', () => {
		const id = addToast({ message: 'Info toast' });
		const toasts = getToasts();
		const found = toasts.find((t) => t.id === id);
		expect(found!.type).toBe('info');
	});

	it('defaults duration to 4000', () => {
		const id = addToast({ message: 'Timer toast' });
		const toasts = getToasts();
		const found = toasts.find((t) => t.id === id);
		expect(found!.duration).toBe(4000);
	});

	it('accepts custom type', () => {
		const id = addToast({ message: 'Error!', type: 'error' });
		const toasts = getToasts();
		const found = toasts.find((t) => t.id === id);
		expect(found!.type).toBe('error');
	});

	it('accepts success type', () => {
		const id = addToast({ message: 'Done', type: 'success' });
		const found = getToasts().find((t) => t.id === id);
		expect(found!.type).toBe('success');
	});

	it('accepts warning type', () => {
		const id = addToast({ message: 'Warn', type: 'warning' });
		const found = getToasts().find((t) => t.id === id);
		expect(found!.type).toBe('warning');
	});

	it('accepts custom duration', () => {
		const id = addToast({ message: 'Short', duration: 1000 });
		const found = getToasts().find((t) => t.id === id);
		expect(found!.duration).toBe(1000);
	});

	it('auto-removes after duration elapses', () => {
		const id = addToast({ message: 'Auto remove', duration: 2000 });
		expect(getToasts().find((t) => t.id === id)).toBeDefined();
		vi.advanceTimersByTime(2001);
		expect(getToasts().find((t) => t.id === id)).toBeUndefined();
	});

	it('does not auto-remove when duration is 0', () => {
		const id = addToast({ message: 'Persistent', duration: 0 });
		vi.advanceTimersByTime(10000);
		expect(getToasts().find((t) => t.id === id)).toBeDefined();
	});

	it('each toast gets a unique id', () => {
		const id1 = addToast({ message: 'A' });
		const id2 = addToast({ message: 'B' });
		expect(id1).not.toBe(id2);
	});

	it('multiple toasts can coexist', () => {
		addToast({ message: 'First', duration: 0 });
		addToast({ message: 'Second', duration: 0 });
		addToast({ message: 'Third', duration: 0 });
		expect(getToasts().length).toBeGreaterThanOrEqual(3);
	});
});

describe('removeToast', () => {
	it('removes a toast by id', () => {
		const id = addToast({ message: 'Remove me', duration: 0 });
		removeToast(id);
		expect(getToasts().find((t) => t.id === id)).toBeUndefined();
	});

	it('does not throw when removing non-existent id', () => {
		expect(() => removeToast('nonexistent-id')).not.toThrow();
	});

	it('only removes the matching toast, not others', () => {
		const id1 = addToast({ message: 'Keep', duration: 0 });
		const id2 = addToast({ message: 'Remove', duration: 0 });
		removeToast(id2);
		expect(getToasts().find((t) => t.id === id1)).toBeDefined();
		expect(getToasts().find((t) => t.id === id2)).toBeUndefined();
	});
});

describe('getToasts', () => {
	it('returns an array', () => {
		expect(Array.isArray(getToasts())).toBe(true);
	});

	it('returns empty array when no toasts', () => {
		expect(getToasts()).toHaveLength(0);
	});
});
