import { apiGet, apiPost, apiPut, apiDelete } from './client';

// NOTE: apiGet/apiPost/apiPut/apiDelete return Promise<T | null> (null on
// 401/204). This helper UNWRAPS: list() falls back to [], get/create/update throw
// on a null result (a 401 already redirected to /login, so reaching here with
// null is an error). Keeps a non-null Crud contract for callers.
export interface Crud<T, TInput> {
	list(): Promise<T[]>;
	get(id: number): Promise<T>;
	create(input: TInput): Promise<T>;
	update(id: number, input: TInput): Promise<T>;
	remove(id: number): Promise<void>;
}

function must<T>(v: T | null, what: string): T {
	if (v === null) throw new Error(`${what}: no data`);
	return v;
}

export function createCrud<T, TInput>(resource: string): Crud<T, TInput> {
	const base = `/api/${resource}`;
	return {
		list: async () => (await apiGet<T[]>(base)) ?? [],
		get: async (id) => must(await apiGet<T>(`${base}/${id}`), `${resource} get`),
		create: async (input) => must(await apiPost<T>(base, input), `${resource} create`),
		update: async (id, input) => must(await apiPut<T>(`${base}/${id}`, input), `${resource} update`),
		remove: async (id) => {
			await apiDelete<void>(`${base}/${id}`);
		}
	};
}
