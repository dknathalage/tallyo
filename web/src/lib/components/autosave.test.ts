import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createAutosave, type SaveState } from './autosave';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

type Payload = { name: string };
type Row = { id: number; name: string };

function harness(overrides: Partial<{ createImpl: (p: Payload) => Promise<Row> }> = {}) {
	const states: SaveState[] = [];
	const created: number[] = [];
	const create = vi.fn(overrides.createImpl ?? (async (p: Payload) => ({ id: 1, ...p })));
	const update = vi.fn(async (_id: number, p: Payload) => ({ id: 1, ...p }));
	const a = createAutosave<Payload, Row>({
		create,
		update,
		delay: 400,
		onState: (s) => states.push(s),
		onCreated: (id) => created.push(id)
	});
	return { a, create, update, states, created };
}

describe('createAutosave', () => {
	it('coalesces rapid edits into a single save', async () => {
		const { a, create } = harness();
		a.schedule({ name: 'a' });
		a.schedule({ name: 'b' });
		vi.advanceTimersByTime(400);
		await vi.runAllTimersAsync();
		expect(create).toHaveBeenCalledTimes(1);
		expect(create).toHaveBeenCalledWith({ name: 'b' });
	});

	it('creates once, then updates with the captured id', async () => {
		const { a, create, update, created } = harness();
		a.schedule({ name: 'a' });
		await vi.runAllTimersAsync();
		expect(create).toHaveBeenCalledTimes(1);
		expect(created).toEqual([1]);
		a.schedule({ name: 'b' });
		await vi.runAllTimersAsync();
		expect(update).toHaveBeenCalledWith(1, { name: 'b' });
		expect(create).toHaveBeenCalledTimes(1);
	});

	it('serializes a mid-flight edit into one follow-up save', async () => {
		let resolveCreate!: (r: Row) => void;
		const createImpl = (_p: Payload) => new Promise<Row>((res) => (resolveCreate = res));
		const { a, update } = harness({ createImpl });
		a.schedule({ name: 'a' });
		await vi.advanceTimersByTimeAsync(400); // flush → create in flight
		a.schedule({ name: 'b' }); // arrives mid-flight
		await vi.advanceTimersByTimeAsync(400);
		resolveCreate({ id: 1, name: 'a' });
		await vi.runAllTimersAsync();
		expect(update).toHaveBeenCalledTimes(1);
		expect(update).toHaveBeenCalledWith(1, { name: 'b' });
	});

	it('reports error then retries the failed payload', async () => {
		const create = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({ id: 1, name: 'a' });
		const states: SaveState[] = [];
		const a = createAutosave<Payload, Row>({
			create,
			update: vi.fn(async (id, p) => ({ id, ...p })),
			delay: 400,
			onState: (s) => states.push(s)
		});
		a.schedule({ name: 'a' });
		await vi.runAllTimersAsync();
		expect(states).toContain('error');
		a.retry();
		await vi.runAllTimersAsync();
		expect(create).toHaveBeenCalledTimes(2);
		expect(states).toContain('saved');
	});
});
