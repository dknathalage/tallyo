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

import { computeChanges } from '$lib/db/audit.js';
import type { PayerRepository } from '../interfaces/PayerRepository.js';
import type { AuditRepository } from '../interfaces/AuditRepository.js';
import type { StorageTransaction } from '../interfaces/StorageTransaction.js';
import type { CreatePayerInput, UpdatePayerInput } from '../interfaces/types.js';
import type { Payer, Client, PartySnapshot } from '$lib/types/index.js';

export class SqlitePayerRepository implements PayerRepository {
	constructor(
		private readonly _audit: AuditRepository,
		private readonly _tx: StorageTransaction
	) {}

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

	async createPayer(data: CreatePayerInput): Promise<number> {
		const id = await createPayer(data);
		this._audit.logAudit({
			entity_type: 'payer',
			entity_id: id,
			action: 'create',
			changes: {
				name: { old: null, new: data.name },
				email: { old: null, new: data.email ?? '' }
			}
		});
		return id;
	}

	async updatePayer(id: number, data: UpdatePayerInput): Promise<void> {
		const oldPayer = getPayer(id);
		await updatePayer(id, data);
		if (oldPayer) {
			const changes = computeChanges(
				oldPayer as unknown as Record<string, unknown>,
				{ name: data.name, email: data.email ?? '', phone: data.phone ?? '', address: data.address ?? '', metadata: data.metadata ?? '{}' },
				['name', 'email', 'phone', 'address', 'metadata']
			);
			if (Object.keys(changes).length > 0) {
				this._audit.logAudit({ entity_type: 'payer', entity_id: id, action: 'update', changes });
			}
		}
	}

	async deletePayer(id: number): Promise<void> {
		const payer = getPayer(id);
		await deletePayer(id);
		this._audit.logAudit({ entity_type: 'payer', entity_id: id, action: 'delete', context: payer?.name ?? '' });
	}

	async bulkDeletePayers(ids: number[]): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const payers = ids.map((id) => getPayer(id));
		await bulkDeletePayers(ids);
		for (let i = 0; i < ids.length; i++) {
			this._audit.logAudit({ entity_type: 'payer', entity_id: ids[i], action: 'delete', context: payers[i]?.name ?? '', batch_id });
		}
	}
}
