import { addToast } from '$lib/stores/toast.svelte.js';

type ApiErrorBody = { message?: string };

export async function apiFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
	const res = await fetch(input, init);
	if (!res.ok) {
		let message = `Request failed (${res.status})`;
		try {
			const body = (await res.clone().json()) as ApiErrorBody;
			if (body.message) message = body.message;
		} catch {
			const text = await res.clone().text().catch(() => '');
			if (text) message = text;
		}
		addToast({ message, type: 'error' });
	}
	return res;
}
