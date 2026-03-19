import { getDashboardStats, getMonthlyRevenue } from '$lib/db/queries/dashboard.js';
import type { DashboardRepository } from '../interfaces/DashboardRepository.js';
import type { DashboardStats, MonthlyRevenue } from '$lib/types/index.js';

export class PgDashboardRepository implements DashboardRepository {
	async getDashboardStats(): Promise<DashboardStats> {
		return await getDashboardStats();
	}

	async getMonthlyRevenue(): Promise<MonthlyRevenue[]> {
		return await getMonthlyRevenue();
	}
}
