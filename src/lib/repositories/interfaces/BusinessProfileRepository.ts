import type { BusinessProfile, PartySnapshot } from '$lib/types/index.js';
import type { SaveBusinessProfileInput } from './types.js';

export interface BusinessProfileRepository {
	getBusinessProfile(): Promise<BusinessProfile | null>;
	buildBusinessSnapshot(): Promise<PartySnapshot>;

	saveBusinessProfile(data: SaveBusinessProfileInput): Promise<void>;
}
