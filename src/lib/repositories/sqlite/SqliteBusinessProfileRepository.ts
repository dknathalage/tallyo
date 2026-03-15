import {
	getBusinessProfile,
	saveBusinessProfile,
	buildBusinessSnapshot
} from '$lib/db/queries/business-profile.js';
import type { BusinessProfileRepository } from '../interfaces/BusinessProfileRepository.js';
import type { SaveBusinessProfileInput } from '../interfaces/types.js';
import type { BusinessProfile, PartySnapshot } from '$lib/types/index.js';

export class SqliteBusinessProfileRepository implements BusinessProfileRepository {
	getBusinessProfile(): BusinessProfile | null {
		return getBusinessProfile();
	}

	buildBusinessSnapshot(): PartySnapshot {
		return buildBusinessSnapshot();
	}

	async saveBusinessProfile(data: SaveBusinessProfileInput): Promise<void> {
		return saveBusinessProfile(data);
	}
}
