import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async ({ url }) => {
	const page = parseInt(url.searchParams.get('page') || '1', 10);
	const limit = Math.min(parseInt(url.searchParams.get('limit') || '50', 10), 200);
	return {
		clientsResult: await repositories.clients.getClients(undefined, { page, limit }),
		rateTiers: await repositories.rateTiers.getRateTiers()
	};
};
