import { apiGet, apiPut, tenantPath } from '$lib/api/client';
import { onEntity } from '$lib/realtime/events';

export interface BusinessProfile {
	name: string;
	email: string;
	phone: string;
	address: string;
	logo: string;
	metadata: string;
	defaultCurrency: string;
}

export type BusinessProfileInput = Partial<BusinessProfile>;

const EMPTY: BusinessProfile = {
	name: '',
	email: '',
	phone: '',
	address: '',
	logo: '',
	metadata: '',
	defaultCurrency: ''
};

function normalize(raw: Partial<BusinessProfile> | null): BusinessProfile {
	if (raw === null) return { ...EMPTY };
	return {
		name: raw.name ?? '',
		email: raw.email ?? '',
		phone: raw.phone ?? '',
		address: raw.address ?? '',
		logo: raw.logo ?? '',
		metadata: raw.metadata ?? '',
		defaultCurrency: raw.defaultCurrency ?? ''
	};
}

function createProfileStore() {
	let profile = $state<BusinessProfile>({ ...EMPTY });
	let loading = $state(false);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let subscribed = false;

	async function load(): Promise<void> {
		loading = true;
		error = null;
		try {
			const data = await apiGet<Partial<BusinessProfile> | null>(tenantPath('business-profile'));
			profile = normalize(data);
		} catch (e) {
			error = e instanceof Error ? e.message : 'failed to load profile';
		} finally {
			loading = false;
		}
	}

	async function save(input: BusinessProfileInput): Promise<void> {
		if (input === null || typeof input !== 'object') {
			throw new Error('save: input must be an object');
		}
		saving = true;
		error = null;
		try {
			const body = { ...profile, ...input };
			await apiPut<Partial<BusinessProfile>>(tenantPath('business-profile'), body);
			// SSE echo will also trigger load(); reload now for immediacy.
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'failed to save profile';
			throw e;
		} finally {
			saving = false;
		}
	}

	/** Subscribe to SSE invalidations exactly once (browser only). */
	function subscribe(): void {
		if (subscribed) return;
		subscribed = true;
		onEntity('business_profile', () => {
			void load();
		});
	}

	return {
		get profile() {
			return profile;
		},
		get loading() {
			return loading;
		},
		get saving() {
			return saving;
		},
		get error() {
			return error;
		},
		load,
		save,
		subscribe
	};
}

export const businessProfile = createProfileStore();
