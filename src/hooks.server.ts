import type { HandleServerError } from '@sveltejs/kit';
import { ensureMigrations } from '$lib/db/connection';

ensureMigrations();

export const handleError: HandleServerError = ({ error, event }) => {
	const message = error instanceof Error ? error.message : String(error);
	console.error(`[${event.request.method}] ${event.url.pathname} →`, message);
	return { message: 'Internal server error' };
};
