import { json } from '@sveltejs/kit';
import { getDb } from '$lib/db/connection';

export function GET() {
	try {
		const db = getDb();
		db.prepare('SELECT 1').get();
		return json({ status: 'ok', db: 'connected' });
	} catch (e) {
		return json({ status: 'error', message: String(e) }, { status: 503 });
	}
}
