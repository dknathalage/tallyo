import { execute, query, save } from '../connection.svelte.js';
import { logAudit } from '../audit.js';
import type { BusinessProfile, PartySnapshot } from '../../types/index.js';

export function getBusinessProfile(): BusinessProfile | null {
	const results = query<BusinessProfile>('SELECT * FROM business_profile WHERE id = 1');
	return results.length > 0 ? results[0] : null;
}

export async function saveBusinessProfile(data: {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
	logo?: string;
	metadata?: string;
	default_currency?: string;
}): Promise<void> {
	const existing = getBusinessProfile();
	execute(
		`INSERT OR REPLACE INTO business_profile (id, uuid, name, email, phone, address, logo, metadata, default_currency, updated_at) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		[
			existing?.uuid ?? crypto.randomUUID(),
			data.name,
			data.email ?? '',
			data.phone ?? '',
			data.address ?? '',
			data.logo ?? '',
			data.metadata ?? '{}',
			data.default_currency ?? existing?.default_currency ?? 'USD'
		]
	);
	logAudit({
		entity_type: 'business_profile',
		entity_id: 1,
		action: existing ? 'update' : 'create',
		context: data.name
	});
	await save();
}

export function buildBusinessSnapshot(): PartySnapshot {
	const profile = getBusinessProfile();
	if (!profile) {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
	let metadata: Record<string, string> = {};
	try { metadata = JSON.parse(profile.metadata); } catch {}
	return {
		name: profile.name,
		email: profile.email,
		phone: profile.phone,
		address: profile.address,
		logo: profile.logo || undefined,
		metadata
	};
}
