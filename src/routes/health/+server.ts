import { json } from '@sveltejs/kit';
import { healthCheck } from '$lib/db/connection';

export function GET() {
	try {
		healthCheck();
		return json({ status: 'ok', db: 'connected' });
	} catch (e) {
		return json({ status: 'error', message: String(e) }, { status: 503 });
	}
}
