import {
	getRateTiers,
	getRateTier,
	getDefaultTier,
	createRateTier,
	updateRateTier,
	deleteRateTier
} from '$lib/db/queries/rate-tiers.js';
import type { RateTierRepository } from '../interfaces/RateTierRepository.js';
import type { CreateRateTierInput, UpdateRateTierInput } from '../interfaces/types.js';
import type { RateTier } from '$lib/types/index.js';

export class SqliteRateTierRepository implements RateTierRepository {
	async getRateTiers(): Promise<RateTier[]> {
		return await getRateTiers();
	}

	async getRateTier(id: number): Promise<RateTier | null> {
		return await getRateTier(id);
	}

	async getDefaultTier(): Promise<RateTier | null> {
		return await getDefaultTier();
	}

	async createRateTier(data: CreateRateTierInput): Promise<number> {
		return await createRateTier(data);
	}

	async updateRateTier(id: number, data: UpdateRateTierInput): Promise<void> {
		return await updateRateTier(id, data);
	}

	async deleteRateTier(id: number): Promise<void> {
		return await deleteRateTier(id);
	}
}
