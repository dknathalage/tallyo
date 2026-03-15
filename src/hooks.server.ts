import type { HandleServerError } from '@sveltejs/kit';

/**
 * Global server error handler.
 * Converts known SQLite constraint errors into proper HTTP responses
 * instead of generic 500 Internal Errors.
 */
export const handleError: HandleServerError = ({ error, event }) => {
	const message = error instanceof Error ? error.message : String(error);

	// Log for server-side visibility
	console.error(`[${event.request.method}] ${event.url.pathname} →`, message);

	// SQLite unique constraint → 409 Conflict
	if (message.includes('UNIQUE constraint failed')) {
		const field = message.split('UNIQUE constraint failed: ')[1]?.split('.')[1] ?? 'field';
		return {
			message: `A record with this ${field} already exists`,
			code: 'CONFLICT'
		};
	}

	// SQLite foreign key constraint → 400 Bad Request
	if (message.includes('FOREIGN KEY constraint failed')) {
		return {
			message: 'Referenced record does not exist',
			code: 'BAD_REQUEST'
		};
	}

	// SQLite not null constraint → 400 Bad Request
	if (message.includes('NOT NULL constraint failed')) {
		const field = message.split('NOT NULL constraint failed: ')[1]?.split('.')[1] ?? 'field';
		return {
			message: `${field} is required`,
			code: 'BAD_REQUEST'
		};
	}

	// Business logic errors (thrown explicitly in repositories)
	if (message.includes('is required') || message.includes('Cannot delete')) {
		return { message, code: 'BAD_REQUEST' };
	}

	// Default
	return { message: 'Internal server error', code: 'INTERNAL_ERROR' };
};
