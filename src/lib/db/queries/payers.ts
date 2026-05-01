import { getDb } from '../connection.js';
import { payers, clients } from '../drizzle-schema.js';
import { eq, or, like, asc, inArray } from 'drizzle-orm';
import type { Payer, Client, PartySnapshot } from '../../types/index.js';

export async function getPayers(search?: string): Promise<Payer[]> {
	const db = getDb();
	let q = db.select().from(payers).$dynamic();
	if (search) {
		q = q.where(or(like(payers.name, `%${search}%`), like(payers.email, `%${search}%`)));
	}
	const rows = await q.orderBy(asc(payers.name));
	return rows.map(mapRow);
}

export async function getPayer(id: number): Promise<Payer | null> {
	const db = getDb();
	const rows = await db.select().from(payers).where(eq(payers.id, id));
	const first = rows[0];
	return first ? mapRow(first) : null;
}

export async function createPayer(data: {
	name: string;
	email?: string;
	phone?: string;
	address?: string;
	metadata?: string;
}): Promise<number> {
	if (!data.name.trim()) {
		throw new Error('Payer name is required');
	}
	const db = getDb();
	const result = await db
		.insert(payers)
		.values({
			name: data.name,
			email: data.email ?? '',
			phone: data.phone ?? '',
			address: data.address ?? '',
			metadata: data.metadata ?? '{}'
		})
		.returning({ id: payers.id });
	const inserted = result[0];
	if (!inserted) throw new Error('Failed to insert payer');
	return inserted.id;
}

export async function updatePayer(
	id: number,
	data: { name: string; email?: string; phone?: string; address?: string; metadata?: string }
): Promise<void> {
	if (!data.name.trim()) {
		throw new Error('Payer name is required');
	}
	const db = getDb();
	await db
		.update(payers)
		.set({
			name: data.name,
			email: data.email ?? '',
			phone: data.phone ?? '',
			address: data.address ?? '',
			metadata: data.metadata ?? '{}',
			updated_at: new Date().toISOString()
		})
		.where(eq(payers.id, id));
}

export async function deletePayer(id: number): Promise<void> {
	const db = getDb();
	await db.delete(payers).where(eq(payers.id, id));
}

export async function bulkDeletePayers(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const db = getDb();
	await db.delete(payers).where(inArray(payers.id, ids));
}

export async function getPayerClients(payerId: number): Promise<Client[]> {
	const db = getDb();
	const rows = await db
		.select()
		.from(clients)
		.where(eq(clients.payer_id, payerId))
		.orderBy(asc(clients.name));
	return rows.map((r) => ({
		id: r.id,
		uuid: r.uuid ?? '',
		name: r.name,
		email: r.email ?? '',
		phone: r.phone ?? '',
		address: r.address ?? '',
		pricing_tier_id: r.pricing_tier_id,
		metadata: r.metadata ?? '{}',
		payer_id: r.payer_id,
		created_at: r.created_at ?? '',
		updated_at: r.updated_at ?? ''
	}));
}

export async function buildPayerSnapshot(payerId: number | null): Promise<PartySnapshot> {
	if (!payerId) {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
	const payer = await getPayer(payerId);
	if (!payer) {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
	let metadata: Record<string, string> = {};
	try {
		metadata = JSON.parse(payer.metadata);
	} catch (e) {
		console.error('Failed to parse payer metadata', e);
	}
	return {
		name: payer.name,
		email: payer.email,
		phone: payer.phone,
		address: payer.address,
		metadata
	};
}

function mapRow(row: typeof payers.$inferSelect): Payer {
	return {
		id: row.id,
		uuid: row.uuid,
		name: row.name,
		email: row.email ?? '',
		phone: row.phone ?? '',
		address: row.address ?? '',
		metadata: row.metadata ?? '{}',
		created_at: row.created_at ?? '',
		updated_at: row.updated_at ?? ''
	};
}
