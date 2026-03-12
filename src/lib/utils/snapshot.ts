import type { PartySnapshot } from '$lib/types/index.js';

export type { PartySnapshot };

export function parseSnapshot(json: string): PartySnapshot {
	try {
		const parsed = JSON.parse(json || '{}');
		return {
			name: parsed.name || '',
			email: parsed.email || '',
			phone: parsed.phone || '',
			address: parsed.address || '',
			logo: parsed.logo,
			metadata: parsed.metadata || {}
		};
	} catch {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
}
