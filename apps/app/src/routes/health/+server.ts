import { json } from '@sveltejs/kit';
import { healthCheck } from '$lib/db/connection';

export async function GET() {
	try {
		await healthCheck();
		return json({ status: 'ok', db: 'connected' });
	} catch (e) {
		return json({ status: 'error', message: String(e) }, { status: 503 });
	}
}
