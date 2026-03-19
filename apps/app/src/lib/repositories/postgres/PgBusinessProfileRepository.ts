import {
	getBusinessProfile,
	saveBusinessProfile,
	buildBusinessSnapshot
} from '$lib/db/queries/business-profile.js';
import type { BusinessProfileRepository } from '../interfaces/BusinessProfileRepository.js';
import type { SaveBusinessProfileInput } from '../interfaces/types.js';
import type { BusinessProfile, PartySnapshot } from '$lib/types/index.js';

export class PgBusinessProfileRepository implements BusinessProfileRepository {
	async getBusinessProfile(): Promise<BusinessProfile | null> {
		return await getBusinessProfile();
	}

	async buildBusinessSnapshot(): Promise<PartySnapshot> {
		return await buildBusinessSnapshot();
	}

	async saveBusinessProfile(data: SaveBusinessProfileInput): Promise<void> {
		return await saveBusinessProfile(data);
	}
}
