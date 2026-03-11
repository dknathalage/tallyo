import {
	getPayers,
	getPayer,
	createPayer,
	updatePayer,
	deletePayer,
	bulkDeletePayers,
	getPayerClients,
	buildPayerSnapshot
} from '$lib/db/queries/payers.js';
import type { PayerRepository } from '../interfaces/PayerRepository.js';
import type { CreatePayerInput, UpdatePayerInput } from '../interfaces/types.js';
import type { Payer, Client, PartySnapshot } from '$lib/types/index.js';

export class SqlitePayerRepository implements PayerRepository {
	getPayers(search?: string): Payer[] {
		return getPayers(search);
	}

	getPayer(id: number): Payer | null {
		return getPayer(id);
	}

	getPayerClients(payerId: number): Client[] {
		return getPayerClients(payerId);
	}

	buildPayerSnapshot(payerId: number | null): PartySnapshot {
		return buildPayerSnapshot(payerId);
	}

	createPayer(data: CreatePayerInput): Promise<number> {
		return createPayer(data);
	}

	updatePayer(id: number, data: UpdatePayerInput): Promise<void> {
		return updatePayer(id, data);
	}

	deletePayer(id: number): Promise<void> {
		return deletePayer(id);
	}

	bulkDeletePayers(ids: number[]): Promise<void> {
		return bulkDeletePayers(ids);
	}
}
