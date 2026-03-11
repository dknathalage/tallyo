import { getDashboardStats } from '$lib/db/queries/dashboard.js';
import type { DashboardRepository } from '../interfaces/DashboardRepository.js';
import type { DashboardStats } from '$lib/types/index.js';

export class SqliteDashboardRepository implements DashboardRepository {
	getDashboardStats(): DashboardStats {
		return getDashboardStats();
	}
}
