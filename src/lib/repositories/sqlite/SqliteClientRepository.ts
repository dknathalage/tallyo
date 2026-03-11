import {
	getClients,
	getClient,
	createClient,
	updateClient,
	deleteClient,
	bulkDeleteClients,
	buildClientSnapshot
} from '$lib/db/queries/clients.js';
import type { ClientRepository } from '../interfaces/ClientRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { StorageTransaction } from '../interfaces/StorageTransaction.js';
import type { CreateClientInput, UpdateClientInput } from '../interfaces/types.js';
import type { Client, PartySnapshot } from '$lib/types/index.js';

export class SqliteClientRepository implements ClientRepository {
	constructor(
		private readonly _audit?: AuditRepository,
		private readonly _tx?: StorageTransaction
	) {}

	getClients(search?: string): Client[] {
		return getClients(search);
	}

	getClient(id: number): Client | null {
		return getClient(id);
	}

	buildClientSnapshot(clientId: number): PartySnapshot {
		return buildClientSnapshot(clientId);
	}

	createClient(data: CreateClientInput): Promise<number> {
		return createClient(data);
	}

	updateClient(id: number, data: UpdateClientInput): Promise<void> {
		return updateClient(id, data);
	}

	deleteClient(id: number): Promise<void> {
		return deleteClient(id);
	}

	bulkDeleteClients(ids: number[]): Promise<void> {
		return bulkDeleteClients(ids);
	}
}
