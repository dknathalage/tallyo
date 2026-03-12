import type { DiffResult } from './diff-catalog.js';
import { execute, query, runRaw, save } from '$lib/db/connection.svelte.js';
import { logAudit } from '$lib/db/audit.js';

export async function commitCatalogImport(
	diff: DiffResult,
	options: { updateExisting: boolean }
): Promise<{ inserted: number; updated: number; batchId: string }> {
	const batchId = crypto.randomUUID();
	let inserted = 0;
	let updated = 0;

	try {
		runRaw('BEGIN TRANSACTION');

		for (const row of diff.newItems) {
			execute(
				`INSERT INTO catalog_items (uuid, name, rate, unit, category, sku, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				[
					crypto.randomUUID(),
					row.name,
					row.rate,
					row.unit,
					row.category,
					row.sku,
					Object.keys(row.metadata).length > 0 ? JSON.stringify(row.metadata) : '{}'
				]
			);
			const result = query<{ id: number }>(`SELECT last_insert_rowid() as id`);
			const newId = result[0].id;

			for (const [tierId, tierRate] of Object.entries(row.tierRates)) {
				execute(
					`INSERT OR REPLACE INTO catalog_item_rates (catalog_item_id, rate_tier_id, rate) VALUES (?, ?, ?)`,
					[newId, Number(tierId), tierRate]
				);
			}

			logAudit({
				entity_type: 'catalog',
				entity_id: newId,
				action: 'import',
				changes: {
					name: { old: null, new: row.name },
					rate: { old: null, new: row.rate },
					sku: { old: null, new: row.sku }
				},
				batch_id: batchId
			});

			inserted++;
		}

		if (options.updateExisting) {
			for (const item of diff.updatedItems) {
				const row = item.incoming;
				execute(
					`UPDATE catalog_items SET name = ?, rate = ?, unit = ?, category = ?, metadata = ?, updated_at = datetime('now') WHERE id = ?`,
					[
						row.name,
						row.rate,
						row.unit,
						row.category,
						Object.keys(row.metadata).length > 0 ? JSON.stringify(row.metadata) : '{}',
						item.existing.id
					]
				);

				for (const [tierId, tierRate] of Object.entries(row.tierRates)) {
					execute(
						`INSERT OR REPLACE INTO catalog_item_rates (catalog_item_id, rate_tier_id, rate) VALUES (?, ?, ?)`,
						[item.existing.id, Number(tierId), tierRate]
					);
				}

				logAudit({
					entity_type: 'catalog',
					entity_id: item.existing.id,
					action: 'import',
					changes: {
						name: { old: item.existing.name, new: row.name },
						rate: { old: item.existing.rate, new: row.rate }
					},
					batch_id: batchId
				});

				updated++;
			}
		}

		runRaw('COMMIT');
	} catch (err) {
		try {
			runRaw('ROLLBACK');
		} catch {
			// ignore rollback errors
		}
		throw err;
	}

	await save();

	return { inserted, updated, batchId };
}
