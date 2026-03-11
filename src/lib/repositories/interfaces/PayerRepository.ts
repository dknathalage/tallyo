import type { Payer, Client, PartySnapshot } from '$lib/types/index.js';
import type { CreatePayerInput, UpdatePayerInput } from './types.js';

export interface PayerRepository {
	getPayers(search?: string): Payer[];
	getPayer(id: number): Payer | null;
	getPayerClients(payerId: number): Client[];
	buildPayerSnapshot(payerId: number | null): PartySnapshot;

	createPayer(data: CreatePayerInput): Promise<number>;
	updatePayer(id: number, data: UpdatePayerInput): Promise<void>;
	deletePayer(id: number): Promise<void>;

	bulkDeletePayers(ids: number[]): Promise<void>;
}
