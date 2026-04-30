import { getDb } from '../connection.js';
import { businessProfile, auditLog } from '../drizzle-schema.js';
import { eq } from 'drizzle-orm';
import { logAudit } from '../audit.js';
import type { BusinessProfile, PartySnapshot } from '../../types/index.js';

export async function getBusinessProfile(): Promise<BusinessProfile | null> {
	const db = getDb();
	const rows = await db.select().from(businessProfile).where(eq(businessProfile.id, 1));
	if (rows.length === 0) return null;
	return mapRow(rows[0]);
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
	const db = getDb();
	const existing = await getBusinessProfile();
	await db
		.insert(businessProfile)
		.values({
			id: 1,
			uuid: existing?.uuid ?? crypto.randomUUID(),
			name: data.name,
			email: data.email ?? '',
			phone: data.phone ?? '',
			address: data.address ?? '',
			logo: data.logo ?? '',
			metadata: data.metadata ?? '{}',
			default_currency: data.default_currency ?? existing?.default_currency ?? 'USD',
			updated_at: new Date().toISOString()
		})
		.onConflictDoUpdate({
			target: businessProfile.id,
			set: {
				name: data.name,
				email: data.email ?? '',
				phone: data.phone ?? '',
				address: data.address ?? '',
				logo: data.logo ?? '',
				metadata: data.metadata ?? '{}',
				default_currency: data.default_currency ?? existing?.default_currency ?? 'USD',
				updated_at: new Date().toISOString()
			}
		});
	await logAudit({
		entity_type: 'business_profile',
		entity_id: 1,
		action: existing ? 'update' : 'create',
		context: data.name
	});
}

export async function buildBusinessSnapshot(): Promise<PartySnapshot> {
	const profile = await getBusinessProfile();
	if (!profile) {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
	let metadata: Record<string, string> = {};
	try {
		metadata = JSON.parse(profile.metadata);
	} catch (e) {
		console.error('Failed to parse business profile metadata', e);
	}
	return {
		name: profile.name,
		email: profile.email,
		phone: profile.phone,
		address: profile.address,
		logo: profile.logo || undefined,
		metadata
	};
}

function mapRow(row: typeof businessProfile.$inferSelect): BusinessProfile {
	return {
		id: row.id,
		uuid: row.uuid,
		name: row.name,
		email: row.email ?? '',
		phone: row.phone ?? '',
		address: row.address ?? '',
		logo: row.logo ?? '',
		metadata: row.metadata ?? '{}',
		default_currency: row.default_currency ?? 'USD',
		created_at: (row.created_at as string) ?? '',
		updated_at: (row.updated_at as string) ?? ''
	};
}
