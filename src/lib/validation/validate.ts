import { error } from '@sveltejs/kit';
import type { ZodSchema } from 'zod';

export function validate<T>(schema: ZodSchema<T>, data: unknown): T {
	const result = schema.safeParse(data);
	if (!result.success) {
		const messages = result.error.issues.map(e => `${e.path.join('.')}: ${e.message}`).join(', ');
		throw error(400, `Validation failed: ${messages}`);
	}
	return result.data;
}
