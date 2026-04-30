import type { RateTier } from '$lib/types/index.js';
import type { CreateRateTierInput, UpdateRateTierInput } from './types.js';

export interface RateTierRepository {
	getRateTiers(): Promise<RateTier[]>;
	getRateTier(id: number): Promise<RateTier | null>;
	getDefaultTier(): Promise<RateTier | null>;

	createRateTier(data: CreateRateTierInput): Promise<number>;
	updateRateTier(id: number, data: UpdateRateTierInput): Promise<void>;
	deleteRateTier(id: number): Promise<void>;
}
