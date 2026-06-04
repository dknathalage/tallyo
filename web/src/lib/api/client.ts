import { goto } from '$app/navigation';
import { page } from '$app/state';

/** Shape of an error body returned by the Go API. */
interface ApiErrorBody {
	error?: string;
}

/** Thrown for any non-2xx response (other than the 401 redirect path). */
export class ApiError extends Error {
	readonly status: number;
	constructor(status: number, message: string) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
	}
}

/** Redirect to /login on 401, unless we're already on a public auth page. */
function handleUnauthorized(): void {
	if (typeof window === 'undefined') return;
	const path = page.url?.pathname ?? window.location.pathname;
	const publicPaths = ['/login', '/setup', '/accept-invite'];
	if (publicPaths.some((p) => path === p || path.startsWith(p + '/'))) return;
	void goto('/login');
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T | null> {
	if (typeof path !== 'string' || path.length === 0) {
		throw new Error('api request: path must be a non-empty string');
	}

	const init: RequestInit = {
		method,
		credentials: 'include',
		headers: { 'Content-Type': 'application/json' }
	};
	if (body !== undefined) {
		init.body = JSON.stringify(body);
	}

	const res = await fetch(path, init);

	if (res.status === 401) {
		handleUnauthorized();
		return null;
	}

	if (res.status === 204) {
		return null;
	}

	const text = await res.text();
	const data: unknown = text.length > 0 ? JSON.parse(text) : null;

	if (!res.ok) {
		const errMsg = (data as ApiErrorBody | null)?.error ?? `request failed (${res.status})`;
		throw new ApiError(res.status, errMsg);
	}

	return data as T;
}

export function apiGet<T>(path: string): Promise<T | null> {
	return request<T>('GET', path);
}

export function apiPost<T>(path: string, body?: unknown): Promise<T | null> {
	return request<T>('POST', path, body ?? {});
}

export function apiPut<T>(path: string, body?: unknown): Promise<T | null> {
	return request<T>('PUT', path, body ?? {});
}
