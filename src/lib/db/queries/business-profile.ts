import { getDb } from '../connection.js';
import { businessProfile } from '../drizzle-schema.js';
import { eq } from 'drizzle-orm';
import { logAudit } from '../audit.js';
import type { BusinessProfile, PartySnapshot } from '../../types/index.js';

export async function getBusinessProfile(): Promise<BusinessProfile | null> {
	const db = getDb();
	const rows = await db.select().from(businessProfile).where(eq(businessProfile.id, 1));
	const first = rows[0];
	if (!first) return null;
	return mapRow(first);
}

interface SaveBusinessProfileInput {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
	logo?: string;
	metadata?: string;
	default_currency?: string;
}

function normalizeProfileFields(
	data: SaveBusinessProfileInput,
	existingDefaultCurrency: string | undefined
): {
	name: string;
	email: string;
	phone: string;
	address: string;
	logo: string;
	metadata: string;
	default_currency: string;
	updated_at: string;
} {
	return {
		name: data.name,
		email: data.email ?? '',
		phone: data.phone ?? '',
		address: data.address ?? '',
		logo: data.logo ?? '',
		metadata: data.metadata ?? '{}',
		default_currency: data.default_currency ?? existingDefaultCurrency ?? 'USD',
		updated_at: new Date().toISOString()
	};
}

export async function saveBusinessProfile(data: SaveBusinessProfileInput): Promise<void> {
	const db = getDb();
	const existing = await getBusinessProfile();
	const fields = normalizeProfileFields(data, existing?.default_currency);
	await db
		.insert(businessProfile)
		.values({
			id: 1,
			uuid: existing?.uuid ?? crypto.randomUUID(),
			...fields
		})
		.onConflictDoUpdate({
			target: businessProfile.id,
			set: fields
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
		metadata = JSON.parse(profile.metadata) as Record<string, string>;
	} catch (e) {
		console.error('Failed to parse business profile metadata', e);
	}
	const logo = profile.logo || undefined;
	return {
		name: profile.name,
		email: profile.email,
		phone: profile.phone,
		address: profile.address,
		...(logo !== undefined && { logo }),
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
		created_at: row.created_at ?? '',
		updated_at: row.updated_at ?? ''
	};
}
