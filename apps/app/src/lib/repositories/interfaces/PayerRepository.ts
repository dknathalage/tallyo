import type { Payer, Client, PartySnapshot } from '$lib/types/index.js';
import type { CreatePayerInput, UpdatePayerInput } from './types.js';

export interface PayerRepository {
	getPayers(search?: string): Promise<Payer[]>;
	getPayer(id: number): Promise<Payer | null>;
	getPayerClients(payerId: number): Promise<Client[]>;
	buildPayerSnapshot(payerId: number | null): Promise<PartySnapshot>;

	createPayer(data: CreatePayerInput): Promise<number>;
	updatePayer(id: number, data: UpdatePayerInput): Promise<void>;
	deletePayer(id: number): Promise<void>;

	bulkDeletePayers(ids: number[]): Promise<void>;
}
