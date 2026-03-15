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
	getRateTiers(): RateTier[] {
		return getRateTiers();
	}

	getRateTier(id: number): RateTier | null {
		return getRateTier(id);
	}

	getDefaultTier(): RateTier | null {
		return getDefaultTier();
	}

	async createRateTier(data: CreateRateTierInput): Promise<number> {
		return createRateTier(data);
	}

	async updateRateTier(id: number, data: UpdateRateTierInput): Promise<void> {
		return updateRateTier(id, data);
	}

	async deleteRateTier(id: number): Promise<void> {
		return deleteRateTier(id);
	}
}
