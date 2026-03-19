import type { Client, PartySnapshot, ClientRevenueSummary, PaginationParams, PaginatedResult } from '$lib/types/index.js';
import type { CreateClientInput, UpdateClientInput } from './types.js';

export interface ClientRepository {
	getClients(search?: string, pagination?: PaginationParams): Promise<PaginatedResult<Client>>;
	getClient(id: number): Promise<Client | null>;
	buildClientSnapshot(clientId: number): Promise<PartySnapshot>;
	getClientRevenueSummary(clientId: number): Promise<ClientRevenueSummary>;

	createClient(data: CreateClientInput): Promise<number>;
	updateClient(id: number, data: UpdateClientInput): Promise<void>;
	deleteClient(id: number): Promise<void>;

	bulkDeleteClients(ids: number[]): Promise<void>;
}
