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
import type { CreatePayerInput, UpdatePayerInput } from '../interfaces/types.js';
import type { Payer, Client, PartySnapshot } from '$lib/types/index.js';

export class PgPayerRepository implements PayerRepository {
	constructor(private readonly _audit: AuditRepository) {}

	async getPayers(search?: string): Promise<Payer[]> {
		return await getPayers(search);
	}

	async getPayer(id: number): Promise<Payer | null> {
		return await getPayer(id);
	}

	async getPayerClients(payerId: number): Promise<Client[]> {
		return await getPayerClients(payerId);
	}

	async buildPayerSnapshot(payerId: number | null): Promise<PartySnapshot> {
		return await buildPayerSnapshot(payerId);
	}

	async createPayer(data: CreatePayerInput): Promise<number> {
		const id = await createPayer(data);
		await this._audit.logAudit({
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
		const oldPayer = await getPayer(id);
		await updatePayer(id, data);
		if (oldPayer) {
			const changes = computeChanges(
				oldPayer as unknown as Record<string, unknown>,
				{ name: data.name, email: data.email ?? '', phone: data.phone ?? '', address: data.address ?? '', metadata: data.metadata ?? '{}' },
				['name', 'email', 'phone', 'address', 'metadata']
			);
			if (Object.keys(changes).length > 0) {
				await this._audit.logAudit({ entity_type: 'payer', entity_id: id, action: 'update', changes });
			}
		}
	}

	async deletePayer(id: number): Promise<void> {
		const payer = await getPayer(id);
		await deletePayer(id);
		await this._audit.logAudit({ entity_type: 'payer', entity_id: id, action: 'delete', context: payer?.name ?? '' });
	}

	async bulkDeletePayers(ids: number[]): Promise<void> {
		if (ids.length === 0) return;
		const batch_id = crypto.randomUUID();
		const payers = await Promise.all(ids.map((id) => getPayer(id)));
		await bulkDeletePayers(ids);
		for (let i = 0; i < ids.length; i++) {
			await this._audit.logAudit({ entity_type: 'payer', entity_id: ids[i], action: 'delete', context: payers[i]?.name ?? '', batch_id });
		}
	}
}
