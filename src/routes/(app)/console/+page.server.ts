import type { PageServerLoad } from './$types';
import { repositories } from '$lib/repositories/sqlite/index.js';

export const load: PageServerLoad = () => {
	return {
		stats: repositories.dashboard.getDashboardStats(),
		monthlyRevenue: repositories.dashboard.getMonthlyRevenue(),
		defaultCurrency: repositories.businessProfile.getBusinessProfile()?.default_currency || 'USD'
	};
};
