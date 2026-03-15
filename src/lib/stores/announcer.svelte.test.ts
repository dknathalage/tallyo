import { describe, it, expect } from 'vitest';
import { announcer } from './announcer.svelte.js';

describe('announcer store', () => {
	it('initializes with empty politeMessage', () => {
		expect(announcer.politeMessage).toBe('');
	});

	it('initializes with empty assertiveMessage', () => {
		expect(announcer.assertiveMessage).toBe('');
	});

	it('announce polite resets politeMessage to empty first', () => {
		announcer.politeMessage = 'old message';
		announcer.announce('new polite message', 'polite');
		// Immediately after call, politeMessage should be '' (reset)
		expect(announcer.politeMessage).toBe('');
	});

	it('announce polite sets politeMessage via microtask', async () => {
		announcer.announce('hello polite', 'polite');
		await Promise.resolve(); // flush microtask
		expect(announcer.politeMessage).toBe('hello polite');
	});

	it('announce assertive resets assertiveMessage to empty first', () => {
		announcer.assertiveMessage = 'old assertive';
		announcer.announce('new assertive', 'assertive');
		// Immediately after call, assertiveMessage should be '' (reset)
		expect(announcer.assertiveMessage).toBe('');
	});

	it('announce assertive sets assertiveMessage via microtask', async () => {
		announcer.announce('urgent!', 'assertive');
		await Promise.resolve();
		expect(announcer.assertiveMessage).toBe('urgent!');
	});

	it('defaults to polite priority when no priority given', async () => {
		announcer.announce('default priority message');
		await Promise.resolve();
		expect(announcer.politeMessage).toBe('default priority message');
	});

	it('polite message does not affect assertiveMessage', async () => {
		announcer.assertiveMessage = 'assertive stays';
		announcer.announce('polite update', 'polite');
		await Promise.resolve();
		// assertiveMessage should not change
		expect(announcer.assertiveMessage).toBe('assertive stays');
	});

	it('assertive message does not affect politeMessage after set', async () => {
		announcer.politeMessage = 'polite stays';
		announcer.announce('assertive update', 'assertive');
		await Promise.resolve();
		expect(announcer.politeMessage).toBe('polite stays');
	});

	it('subsequent polite announces update the message', async () => {
		announcer.announce('first', 'polite');
		await Promise.resolve();
		announcer.announce('second', 'polite');
		// After first microtask but before second, politeMessage is '' (reset)
		expect(announcer.politeMessage).toBe('');
		await Promise.resolve();
		expect(announcer.politeMessage).toBe('second');
	});

	it('can announce empty string', async () => {
		announcer.politeMessage = 'was something';
		announcer.announce('', 'polite');
		await Promise.resolve();
		expect(announcer.politeMessage).toBe('');
	});
});
