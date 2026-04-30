import { getDb } from '../connection.js';
import { clients, rateTiers, payers, invoices } from '../drizzle-schema.js';
import { eq, like, or, inArray, sql } from 'drizzle-orm';
import type {
	Client,
	PartySnapshot,
	ClientRevenueSummary,
	PaginationParams,
	PaginatedResult
} from '../../types/index.js';
import { paginate } from '../../types/index.js';
import type { CreateClientInput } from '../../repositories/interfaces/types.js';
import { getBusinessProfile } from './business-profile.js';

function mapRow(row: Record<string, unknown>): Client {
	return {
		id: row.id as number,
		uuid: row.uuid as string,
		name: row.name as string,
		email: (row.email as string) ?? '',
		phone: (row.phone as string) ?? '',
		address: (row.address as string) ?? '',
		pricing_tier_id: (row.pricing_tier_id as number | null) ?? null,
		pricing_tier_name: (row.pricing_tier_name as string) ?? undefined,
		metadata: (row.metadata as string) ?? '{}',
		payer_id: (row.payer_id as number | null) ?? null,
		payer_name: (row.payer_name as string) ?? undefined,
		created_at: (row.created_at as string) ?? '',
		updated_at: (row.updated_at as string) ?? ''
	};
}

export async function getClients(
	search?: string,
	pagination?: PaginationParams
): Promise<PaginatedResult<Client>> {
	const db = getDb();

	const baseQuery = db
		.select({
			id: clients.id,
			uuid: clients.uuid,
			name: clients.name,
			email: clients.email,
			phone: clients.phone,
			address: clients.address,
			pricing_tier_id: clients.pricing_tier_id,
			pricing_tier_name: rateTiers.name,
			metadata: clients.metadata,
			payer_id: clients.payer_id,
			payer_name: payers.name,
			created_at: clients.created_at,
			updated_at: clients.updated_at
		})
		.from(clients)
		.leftJoin(rateTiers, eq(clients.pricing_tier_id, rateTiers.id))
		.leftJoin(payers, eq(clients.payer_id, payers.id));

	let rows;
	if (search) {
		rows = await baseQuery
			.where(or(like(clients.name, `%${search}%`), like(clients.email, `%${search}%`)))
			.orderBy(clients.name);
	} else {
		rows = await baseQuery.orderBy(clients.name);
	}

	const all = rows.map(mapRow);
	return paginate(all, pagination);
}

export async function getClient(id: number): Promise<Client | null> {
	const db = getDb();

	const rows = await db
		.select({
			id: clients.id,
			uuid: clients.uuid,
			name: clients.name,
			email: clients.email,
			phone: clients.phone,
			address: clients.address,
			pricing_tier_id: clients.pricing_tier_id,
			pricing_tier_name: rateTiers.name,
			metadata: clients.metadata,
			payer_id: clients.payer_id,
			payer_name: payers.name,
			created_at: clients.created_at,
			updated_at: clients.updated_at
		})
		.from(clients)
		.leftJoin(rateTiers, eq(clients.pricing_tier_id, rateTiers.id))
		.leftJoin(payers, eq(clients.payer_id, payers.id))
		.where(eq(clients.id, id));

	return rows.length > 0 ? mapRow(rows[0]) : null;
}

/**
 * Inserts a client and returns the new id.
 */
export async function createClient(data: CreateClientInput): Promise<number> {
	if (!data.name?.trim()) {
		throw new Error('Client name is required');
	}
	const db = getDb();

	const result = await db
		.insert(clients)
		.values({
			uuid: data.uuid ?? crypto.randomUUID(),
			name: data.name,
			email: data.email ?? '',
			phone: data.phone ?? '',
			address: data.address ?? '',
			pricing_tier_id: data.pricing_tier_id ?? null,
			metadata: data.metadata ?? '{}',
			payer_id: data.payer_id ?? null
		})
		.returning({ id: clients.id });

	return result[0].id;
}

/**
 * Updates a client.
 */
export async function updateClient(
	id: number,
	data: {
		name: string;
		email?: string;
		phone?: string;
		address?: string;
		pricing_tier_id?: number | null;
		metadata?: string;
		payer_id?: number | null;
	}
): Promise<void> {
	if (!data.name?.trim()) {
		throw new Error('Client name is required');
	}
	const db = getDb();

	await db
		.update(clients)
		.set({
			name: data.name,
			email: data.email ?? '',
			phone: data.phone ?? '',
			address: data.address ?? '',
			pricing_tier_id: data.pricing_tier_id ?? null,
			metadata: data.metadata ?? '{}',
			payer_id: data.payer_id ?? null,
			updated_at: new Date().toISOString()
		})
		.where(eq(clients.id, id));
}

/**
 * Deletes a client.
 */
export async function deleteClient(id: number): Promise<void> {
	const db = getDb();
	await db.delete(clients).where(eq(clients.id, id));
}

/**
 * Bulk deletes clients.
 */
export async function bulkDeleteClients(ids: number[]): Promise<void> {
	if (ids.length === 0) return;
	const db = getDb();
	await db.delete(clients).where(inArray(clients.id, ids));
}

export async function buildClientSnapshot(clientId: number): Promise<PartySnapshot> {
	const client = await getClient(clientId);
	if (!client) {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
	let metadata: Record<string, string> = {};
	try {
		metadata = JSON.parse(client.metadata || '{}');
	} catch (e) {
		console.error('Failed to parse client metadata', e);
	}
	return {
		name: client.name,
		email: client.email,
		phone: client.phone,
		address: client.address,
		metadata
	};
}

export async function getClientRevenueSummary(
	clientId: number
): Promise<ClientRevenueSummary> {
	const profile = await getBusinessProfile();
	const defaultCurrency = profile?.default_currency || 'USD';

	const db = getDb();
	const result = await db
		.select({
			total_invoiced: sql<number>`COALESCE(SUM(${invoices.total}), 0)`,
			total_paid: sql<number>`COALESCE(SUM(CASE WHEN ${invoices.status} = 'paid' THEN ${invoices.total} ELSE 0 END), 0)`,
			outstanding_balance: sql<number>`COALESCE(SUM(CASE WHEN ${invoices.status} IN ('sent', 'overdue') THEN ${invoices.total} ELSE 0 END), 0)`,
			invoice_count: sql<number>`COUNT(*)`
		})
		.from(invoices)
		.where(
			sql`${invoices.client_id} = ${clientId} AND COALESCE(${invoices.currency_code}, 'USD') = ${defaultCurrency}`
		);

	return {
		total_invoiced: result[0]?.total_invoiced ?? 0,
		total_paid: result[0]?.total_paid ?? 0,
		outstanding_balance: result[0]?.outstanding_balance ?? 0,
		invoice_count: result[0]?.invoice_count ?? 0,
		currency_code: defaultCurrency
	};
}
