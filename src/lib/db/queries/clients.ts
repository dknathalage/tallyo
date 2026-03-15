import { execute, query } from '../connection.js';
import type { Client, PartySnapshot, ClientRevenueSummary, PaginationParams, PaginatedResult } from '../../types/index.js';
import { paginate } from '../../types/index.js';
import type { CreateClientInput } from '../../repositories/interfaces/types.js';
import { getBusinessProfile } from './business-profile.js';

export function getClients(search?: string, pagination?: PaginationParams): PaginatedResult<Client> {
	let all: Client[];
	if (search) {
		all = query<Client>(
			`SELECT c.*, rt.name as pricing_tier_name, p.name as payer_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id LEFT JOIN payers p ON c.payer_id = p.id WHERE c.name LIKE ? OR c.email LIKE ? ORDER BY c.name`,
			[`%${search}%`, `%${search}%`]
		);
	} else {
		all = query<Client>(`SELECT c.*, rt.name as pricing_tier_name, p.name as payer_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id LEFT JOIN payers p ON c.payer_id = p.id ORDER BY c.name`);
	}
	return paginate(all, pagination);
}

export function getClient(id: number): Client | null {
	const results = query<Client>(`SELECT c.*, rt.name as pricing_tier_name, p.name as payer_name FROM clients c LEFT JOIN rate_tiers rt ON c.pricing_tier_id = rt.id LEFT JOIN payers p ON c.payer_id = p.id WHERE c.id = ?`, [id]);
	return results.length > 0 ? results[0] : null;
}

/**
 * Pure SQL: inserts a client and returns the new id.
 * No audit logging, no save().
 */
export function createClient(data: CreateClientInput): number {
	if (!data.name?.trim()) {
		throw new Error('Client name is required');
	}
	execute(
		`INSERT INTO clients (uuid, name, email, phone, address, pricing_tier_id, metadata, payer_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		[data.uuid ?? crypto.randomUUID(), data.name, data.email ?? '', data.phone ?? '', data.address ?? '', data.pricing_tier_id ?? null, data.metadata ?? '{}', data.payer_id ?? null]
	);
	const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
	return result[0].id;
}

/**
 * Pure SQL: updates a client.
 * No audit logging, no save().
 */
export function updateClient(
	id: number,
	data: { name: string; email?: string; phone?: string; address?: string; pricing_tier_id?: number | null; metadata?: string; payer_id?: number | null }
): void {
	if (!data.name?.trim()) {
		throw new Error('Client name is required');
	}
	execute(
		`UPDATE clients SET name = ?, email = ?, phone = ?, address = ?, pricing_tier_id = ?, metadata = ?, payer_id = ?, updated_at = datetime('now') WHERE id = ?`,
		[data.name, data.email ?? '', data.phone ?? '', data.address ?? '', data.pricing_tier_id ?? null, data.metadata ?? '{}', data.payer_id ?? null, id]
	);
}

/**
 * Pure SQL: deletes a client.
 * No transaction management, no audit logging, no save().
 */
export function deleteClient(id: number): void {
	execute(`DELETE FROM clients WHERE id = ?`, [id]);
}

/**
 * Pure SQL: bulk deletes clients.
 * No transaction management, no audit logging, no save().
 */
export function bulkDeleteClients(ids: number[]): void {
	if (ids.length === 0) return;
	const placeholders = ids.map(() => '?').join(',');
	execute(`DELETE FROM clients WHERE id IN (${placeholders})`, ids);
}

export function buildClientSnapshot(clientId: number): PartySnapshot {
	const client = getClient(clientId);
	if (!client) {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
	let metadata: Record<string, string> = {};
	try { metadata = JSON.parse(client.metadata || '{}'); } catch (e) { console.error('Failed to parse client metadata', e); }
	return {
		name: client.name,
		email: client.email,
		phone: client.phone,
		address: client.address,
		metadata
	};
}

export function getClientRevenueSummary(clientId: number): ClientRevenueSummary {
	const profile = getBusinessProfile();
	const defaultCurrency = profile?.default_currency || 'USD';

	const result = query<{
		total_invoiced: number;
		total_paid: number;
		outstanding_balance: number;
		invoice_count: number;
	}>(
		`SELECT
			COALESCE(SUM(total), 0) as total_invoiced,
			COALESCE(SUM(CASE WHEN status = 'paid' THEN total ELSE 0 END), 0) as total_paid,
			COALESCE(SUM(CASE WHEN status IN ('sent', 'overdue') THEN total ELSE 0 END), 0) as outstanding_balance,
			COUNT(*) as invoice_count
		FROM invoices
		WHERE client_id = ? AND COALESCE(currency_code, 'USD') = ?`,
		[clientId, defaultCurrency]
	);

	return {
		total_invoiced: result[0]?.total_invoiced ?? 0,
		total_paid: result[0]?.total_paid ?? 0,
		outstanding_balance: result[0]?.outstanding_balance ?? 0,
		invoice_count: result[0]?.invoice_count ?? 0,
		currency_code: defaultCurrency
	};
}
