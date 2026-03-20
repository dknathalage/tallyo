import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/index.js';

export const load: PageServerLoad = async () => {
	return {
		stats: await repositories.dashboard.getDashboardStats(),
		monthlyRevenue: await repositories.dashboard.getMonthlyRevenue(),
		defaultCurrency: (await repositories.businessProfile.getBusinessProfile())?.default_currency || 'USD'
	};
};
