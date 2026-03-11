import type { DashboardStats } from '$lib/types/index.js';

export interface DashboardRepository {
	getDashboardStats(): DashboardStats;
}
