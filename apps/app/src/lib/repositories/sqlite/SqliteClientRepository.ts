import {
	getClients,
	getClient,
	createClient,
	updateClient,
	deleteClient,
	bulkDeleteClients,
	buildClientSnapshot,
	getClientRevenueSummary
} from '$lib/db/queries/clients.js';

import { computeChanges } from '$lib/db/audit.js';
import type { ClientRepository } from '../interfaces/ClientRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { CreateClientInput, UpdateClientInput } from '../interfaces/types.js';
import type { Client, PartySnapshot, ClientRevenueSummary, PaginationParams, PaginatedResult } from '$lib/types/index.js';

export class SqliteClientRepository implements ClientRepository {
	constructor(private readonly _audit: AuditRepository) {}

	async getClients(search?: string, pagination?: PaginationParams): Promise<PaginatedResult<Client>> {
		return await getClients(search, pagination);
	}

	async getClient(id: number): Promise<Client | null> {
		return await getClient(id);
	}

	async buildClientSnapshot(clientId: number): Promise<PartySnapshot> {
		return await buildClientSnapshot(clientId);
	}

	async getClientRevenueSummary(clientId: number): Promise<ClientRevenueSummary> {
		return await getClientRevenueSummary(clientId);
	}

	async createClient(data: CreateClientInput): Promise<number> {
		const id = await createClient(data);
		await this._audit.logAudit({
			entity_type: 'client',
			entity_id: id,
			action: 'create',
			changes: {
				name: { old: null, new: data.name },
				email: { old: null, new: data.email ?? '' }
			}
		});
		return id;
	}

	async updateClient(id: number, data: UpdateClientInput): Promise<void> {
		const oldClient = await getClient(id);
		await updateClient(id, data);
		if (oldClient) {
			const changes = computeChanges(
				oldClient as unknown as Record<string, unknown>,
				{ name: data.name, email: data.email ?? '', phone: data.phone ?? '', address: data.address ?? '', pricing_tier_id: data.pricing_tier_id ?? null, metadata: data.metadata ?? '{}', payer_id: data.payer_id ?? null },
				['name', 'email', 'phone', 'address', 'pricing_tier_id', 'metadata', 'payer_id']
			);
			if (Object.keys(changes).length > 0) {
				await this._audit.logAudit({ entity_type: 'client', entity_id: id, action: 'update', changes });
			}
		}
	}

	async deleteClient(id: number): Promise<void> {
		const client = await getClient(id);
		await deleteClient(id);
		await this._audit.logAudit({ entity_type: 'client', entity_id: id, action: 'delete', context: client?.name ?? '' });
	}

	async bulkDeleteClients(ids: number[]): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const clients = await Promise.all(ids.map((id) => getClient(id)));
		await bulkDeleteClients(ids);
		for (let i = 0; i < ids.length; i++) {
			await this._audit.logAudit({ entity_type: 'client', entity_id: ids[i], action: 'delete', context: clients[i]?.name ?? '', batch_id });
		}
	}
}
