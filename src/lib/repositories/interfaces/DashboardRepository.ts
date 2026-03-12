import type { DashboardStats, MonthlyRevenue } from '$lib/types/index.js';

export interface DashboardRepository {
	getDashboardStats(): DashboardStats;
	getMonthlyRevenue(): MonthlyRevenue[];
}
