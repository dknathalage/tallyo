import { getDashboardStats, getMonthlyRevenue } from '$lib/db/queries/dashboard.js';
import type { DashboardRepository } from '../interfaces/DashboardRepository.js';
import type { DashboardStats, MonthlyRevenue } from '$lib/types/index.js';

export class SqliteDashboardRepository implements DashboardRepository {
	getDashboardStats(): DashboardStats {
		return getDashboardStats();
	}

	getMonthlyRevenue(): MonthlyRevenue[] {
		return getMonthlyRevenue();
	}
}
