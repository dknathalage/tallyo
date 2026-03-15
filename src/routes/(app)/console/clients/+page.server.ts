import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = ({ url }) => {
	const page = parseInt(url.searchParams.get('page') || '1', 10);
	const limit = Math.min(parseInt(url.searchParams.get('limit') || '50', 10), 200);
	return {
		clientsResult: repositories.clients.getClients(undefined, { page, limit }),
		rateTiers: repositories.rateTiers.getRateTiers()
	};
};
